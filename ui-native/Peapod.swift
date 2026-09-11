// Peapod — native macOS app (SwiftUI window) to manage sandboxes.
// Self-contained: it runs the `peapod` binary bundled inside the .app, so there
// is nothing to configure. Requires OrbStack (or Docker) running.
import SwiftUI
import AppKit

struct Sandbox: Codable, Identifiable {
    let id: String
    let image: String
    let network: String
    let name: String?
    let paused: Bool?
}

struct Stat: Codable {
    let cpu_perc: String
    let mem_usage: String
    let mem_perc: String
}

struct HistoryEntry: Codable {
    let time: String
    let command: String
    let exit_code: Int
    let preview: String?
}

struct Template: Codable {
    let name: String
    let image: String
    let desc: String
}

func peapodBin() -> String {
    if let res = Bundle.main.resourceURL?.appendingPathComponent("peapod").path,
       FileManager.default.isExecutableFile(atPath: res) {
        return res
    }
    if let b = ProcessInfo.processInfo.environment["PEAPOD_BIN"], !b.isEmpty {
        return b
    }
    for p in ["/usr/local/bin/peapod", "/opt/homebrew/bin/peapod"] {
        if FileManager.default.isExecutableFile(atPath: p) { return p }
    }
    return "peapod"
}

func childEnv() -> [String: String] {
    var env = ProcessInfo.processInfo.environment
    env["PATH"] = "/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin:" + (env["PATH"] ?? "")
    return env
}

@discardableResult
func runPeapod(_ args: [String]) -> (out: String, err: String, ok: Bool) {
    let p = Process()
    p.executableURL = URL(fileURLWithPath: peapodBin())
    p.arguments = args
    p.environment = childEnv()
    let outPipe = Pipe(), errPipe = Pipe()
    p.standardOutput = outPipe
    p.standardError = errPipe
    do { try p.run() } catch { return ("", "cannot launch peapod", false) }
    let o = outPipe.fileHandleForReading.readDataToEndOfFile()
    let e = errPipe.fileHandleForReading.readDataToEndOfFile()
    p.waitUntilExit()
    return (String(data: o, encoding: .utf8) ?? "",
            String(data: e, encoding: .utf8) ?? "",
            p.terminationStatus == 0)
}

// The helpers below are free (non-actor) functions so they can run off the main
// thread; the Model dispatches them and publishes results back on the main actor.
func fetchStats(_ id: String) -> Stat? {
    let r = runPeapod(["sandbox", "stats", id, "--json"])
    guard r.ok, let d = r.out.data(using: .utf8) else { return nil }
    return try? JSONDecoder().decode(Stat.self, from: d)
}

func execCmd(_ id: String, _ command: String) -> String {
    let r = runPeapod(["sandbox", "exec", id, "sh", "-lc", command])
    return r.out + r.err
}

func fetchLogs(_ id: String) -> String {
    let r = runPeapod(["sandbox", "logs", id, "--tail", "200"])
    let t = (r.out + r.err).trimmingCharacters(in: .whitespacesAndNewlines)
    return t.isEmpty ? "(sem saída ainda)" : t
}

func fetchHistory(_ id: String) -> [HistoryEntry] {
    let r = runPeapod(["sandbox", "history", id, "--json"])
    guard r.ok, let d = r.out.data(using: .utf8) else { return [] }
    return (try? JSONDecoder().decode([HistoryEntry].self, from: d)) ?? []
}

func fetchTemplates() -> [Template] {
    let r = runPeapod(["templates", "--json"])
    guard r.ok, let d = r.out.data(using: .utf8) else { return [] }
    return (try? JSONDecoder().decode([Template].self, from: d)) ?? []
}

/// Last meaningful line of peapod's stderr, without the "peapod: " prefix.
func shortError(_ err: String) -> String {
    let lines = err.split(separator: "\n").map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty }
    var msg = lines.last ?? ""
    if msg.hasPrefix("peapod: ") { msg = String(msg.dropFirst(8)) }
    if msg.isEmpty { msg = "erro desconhecido" }
    return msg.count > 160 ? String(msg.prefix(160)) + "…" : msg
}

/// True when the failure is the container engine being unreachable (as opposed
/// to, say, a typo in an image name) — the only case that deserves the
/// "OrbStack não está rodando" screen.
func looksLikeEngineDown(_ err: String) -> Bool {
    let e = err.lowercased()
    // Covers the older "Cannot connect to the Docker daemon…" wording, the newer
    // "failed to connect to the docker API… check if the daemon is running", a
    // missing/refused socket, and a runtime that isn't installed at all.
    return ["cannot connect to the docker daemon", "failed to connect to the docker api",
            "daemon is running", "docker.sock", "connection refused", "no such file or directory",
            "no container runtime", "executable file not found", "command not found",
            "cannot launch peapod"].contains { e.contains($0) }
}

/// A sticky, dismissible message (an error or a success note). Unlike `status`,
/// the periodic refresh never overwrites it.
struct Flash {
    let text: String
    let isError: Bool
}

@MainActor
final class Model: ObservableObject {
    @Published var boxes: [Sandbox] = []
    @Published var status: String = "carregando…"
    @Published var engineDown = false
    @Published var busy = false
    @Published var flash: Flash?
    private var refreshing = false
    private var refreshQueued = false

    /// Runs `peapod` off the main thread and delivers the result on the main actor.
    private func run(_ args: [String], _ done: @escaping (String, String, Bool) -> Void) {
        DispatchQueue.global(qos: .userInitiated).async {
            let r = runPeapod(args)
            DispatchQueue.main.async { done(r.out, r.err, r.ok) }
        }
    }

    func refresh() {
        if refreshing { refreshQueued = true; return }
        refreshing = true
        run(["sandbox", "ls", "--json"]) { out, err, ok in
            self.refreshing = false
            defer {
                if self.refreshQueued { self.refreshQueued = false; self.refresh() }
            }
            guard ok, let data = out.data(using: .utf8),
                  let list = try? JSONDecoder().decode([Sandbox].self, from: data) else {
                if looksLikeEngineDown(err) {
                    self.boxes = []
                    self.engineDown = true
                    self.status = "o OrbStack não está rodando"
                } else {
                    self.engineDown = false
                    self.flash = Flash(text: "Falha ao listar sandboxes: " + shortError(err), isError: true)
                }
                return
            }
            self.engineDown = false
            self.boxes = list
            self.status = list.isEmpty ? "nenhum sandbox ainda" : "\(list.count) sandbox(es)"
        }
    }

    func openEngine() {
        let p = Process()
        p.executableURL = URL(fileURLWithPath: "/usr/bin/open")
        p.arguments = ["-a", "OrbStack"]
        try? p.run()
        status = "iniciando o OrbStack…"
    }

    /// Runs a mutating command off the main thread with the busy indicator on,
    /// surfaces a failure as a Flash, and refreshes the list afterwards.
    private func act(_ label: String, _ args: [String], onSuccess: ((String) -> Void)? = nil) {
        if busy { return }
        busy = true
        flash = nil
        run(args) { out, err, ok in
            self.busy = false
            if ok {
                onSuccess?(out)
            } else {
                self.flash = Flash(text: "Falha ao \(label): " + shortError(err), isError: true)
            }
            self.refresh()
        }
    }

    func create(_ image: String) {
        if busy { return }
        let img = image.trimmingCharacters(in: .whitespaces)
        let target = img.isEmpty ? "alpine" : img
        status = "criando sandbox (\(target))… baixando a imagem na primeira vez"
        act("criar o sandbox (\(target))", ["sandbox", "create", target])
    }
    func destroy(_ id: String) { act("apagar \(id)", ["sandbox", "rm", id]) }
    func pause(_ id: String) { act("pausar \(id)", ["sandbox", "pause", id]) }
    func resume(_ id: String) { act("retomar \(id)", ["sandbox", "resume", id]) }

    func snapshot(_ id: String) {
        let name = "\(id)-\(Int(Date().timeIntervalSince1970))"
        act("criar o snapshot de \(id)", ["sandbox", "snapshot", id, name]) { out in
            self.flash = Flash(text: "Snapshot criado: " + out.trimmingCharacters(in: .whitespacesAndNewlines), isError: false)
        }
    }
}

struct Sparkline: View {
    let values: [Double]
    let color: Color
    var body: some View {
        GeometryReader { geo in
            let maxV = max(values.max() ?? 1, 1)
            let w = geo.size.width, h = geo.size.height
            Path { p in
                guard values.count > 1 else { return }
                for (i, v) in values.enumerated() {
                    let x = w * CGFloat(i) / CGFloat(values.count - 1)
                    let y = h - h * CGFloat(v / maxV)
                    if i == 0 { p.move(to: CGPoint(x: x, y: y)) } else { p.addLine(to: CGPoint(x: x, y: y)) }
                }
            }
            .stroke(color, style: StrokeStyle(lineWidth: 1.5, lineJoin: .round))
        }
    }
}

struct DetailView: View {
    let box: Sandbox
    @Environment(\.dismiss) private var dismiss
    @State private var logs = "carregando…"
    @State private var stat: Stat?
    @State private var history: [HistoryEntry] = []
    @State private var cpuHist: [Double] = []
    @State private var memHist: [Double] = []
    @State private var cmd = ""
    @State private var cmdOut = ""
    @State private var running = false
    @State private var tab = 1 // 0 = Logs, 1 = History, 2 = Run
    private let sampleTimer = Timer.publish(every: 2, on: .main, in: .common).autoconnect()

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text(box.id).font(.system(.title3, design: .monospaced))
                Spacer()
                Button("Atualizar") { load() }
                Button("Fechar") { dismiss() }
            }
            Text("\(box.image) · \(box.network)").font(.caption).foregroundColor(.secondary)
            if let s = stat {
                HStack(spacing: 24) {
                    VStack(alignment: .leading, spacing: 2) {
                        Label(s.cpu_perc, systemImage: "cpu").font(.caption)
                        Sparkline(values: cpuHist, color: .green).frame(width: 130, height: 26)
                    }
                    VStack(alignment: .leading, spacing: 2) {
                        Label("\(s.mem_usage) (\(s.mem_perc))", systemImage: "memorychip").font(.caption)
                        Sparkline(values: memHist, color: .blue).frame(width: 130, height: 26)
                    }
                }
            }
            Picker("", selection: $tab) {
                Text("Histórico").tag(1)
                Text("Logs").tag(0)
                Text("Executar").tag(2)
            }
            .pickerStyle(.segmented)
            .labelsHidden()

            if tab == 2 {
                HStack {
                    TextField("comando, ex.: ls -la /work", text: $cmd)
                        .textFieldStyle(.roundedBorder)
                        .onSubmit { runCmd() }
                    Button("Executar") { runCmd() }.disabled(running)
                }
                ScrollView {
                    Text(cmdOut.isEmpty ? "Execute um comando dentro do sandbox." : cmdOut)
                        .font(.system(.caption, design: .monospaced))
                        .textSelection(.enabled)
                        .foregroundColor(cmdOut.isEmpty ? .secondary : .primary)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(8)
                }
                .frame(minHeight: 200)
                .background(Color(nsColor: .textBackgroundColor))
                .cornerRadius(6)
            } else {
                ScrollView {
                    if tab == 0 {
                        Text(logs)
                            .font(.system(.caption, design: .monospaced))
                            .textSelection(.enabled)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(8)
                    } else {
                        VStack(alignment: .leading, spacing: 6) {
                            if history.isEmpty {
                                Text("Nenhum comando registrado ainda.").font(.caption).foregroundColor(.secondary)
                            }
                            ForEach(Array(history.enumerated()), id: \.offset) { _, e in
                                VStack(alignment: .leading, spacing: 1) {
                                    HStack {
                                        Text(e.command).font(.system(.caption, design: .monospaced))
                                        Spacer()
                                        Text("exit \(e.exit_code)")
                                            .font(.caption2)
                                            .foregroundColor(e.exit_code == 0 ? .secondary : .red)
                                    }
                                    if let p = e.preview, !p.isEmpty {
                                        Text(p).font(.caption2).foregroundColor(.secondary).lineLimit(1)
                                    }
                                }
                            }
                        }
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .padding(8)
                    }
                }
                .frame(minHeight: 220)
                .background(Color(nsColor: .textBackgroundColor))
                .cornerRadius(6)
            }
        }
        .padding(16)
        .frame(width: 560, height: 420)
        .onAppear { load() }
        .onReceive(sampleTimer) { _ in sample() }
    }

    private func load() {
        let id = box.id
        DispatchQueue.global(qos: .userInitiated).async {
            let l = fetchLogs(id)
            let h = fetchHistory(id)
            DispatchQueue.main.async {
                logs = l
                history = h
            }
        }
        sample()
    }

    private func sample() {
        let id = box.id
        DispatchQueue.global(qos: .utility).async {
            let s = fetchStats(id)
            DispatchQueue.main.async {
                guard let s = s else { return }
                stat = s
                cpuHist = Array((cpuHist + [pct(s.cpu_perc)]).suffix(40))
                memHist = Array((memHist + [pct(s.mem_perc)]).suffix(40))
            }
        }
    }

    private func pct(_ s: String) -> Double {
        Double(s.replacingOccurrences(of: "%", with: "")) ?? 0
    }

    private func runCmd() {
        let c = cmd.trimmingCharacters(in: .whitespaces)
        if c.isEmpty { return }
        let id = box.id
        running = true
        DispatchQueue.global(qos: .userInitiated).async {
            let out = execCmd(id, c)
            let h = fetchHistory(id)
            DispatchQueue.main.async {
                cmdOut = out.trimmingCharacters(in: .whitespacesAndNewlines)
                if cmdOut.isEmpty { cmdOut = "(sem saída)" }
                running = false
                history = h
            }
        }
    }
}

struct CreatePanel: View {
    let model: Model
    let templates: [Template]
    var onPicked: () -> Void = {}
    @State private var customImage = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Comece com um modelo").font(.headline)
            Text("Clique em um modelo para criar um sandbox isolado.")
                .font(.caption).foregroundColor(.secondary)
            ScrollView {
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 150), spacing: 10)], spacing: 10) {
                    ForEach(Array(templates.enumerated()), id: \.offset) { _, t in
                        let parts = t.desc.components(separatedBy: " — ")
                        Button {
                            model.create(t.image); onPicked()
                        } label: {
                            VStack(spacing: 2) {
                                Text(parts.first ?? t.name).bold()
                                if parts.count > 1 {
                                    Text(parts[1]).font(.caption2)
                                        .foregroundColor(.secondary).lineLimit(1)
                                }
                            }
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, 10)
                        }
                        .buttonStyle(.bordered)
                        .disabled(model.busy)
                    }
                }
                .padding(.top, 2)
            }
            Divider()
            HStack {
                TextField("ou uma imagem personalizada, ex.: python:3.12-bookworm", text: $customImage)
                    .textFieldStyle(.roundedBorder)
                    .onSubmit { createCustom() }
                Button("Criar") { createCustom() }
                    .disabled(model.busy || customImage.trimmingCharacters(in: .whitespaces).isEmpty)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private func createCustom() {
        let img = customImage.trimmingCharacters(in: .whitespaces)
        guard !img.isEmpty else { return }
        model.create(img)
        customImage = ""
        onPicked()
    }
}

struct ContentView: View {
    @StateObject private var model = Model()
    @State private var showCreate = false
    @State private var selected: Sandbox?
    @State private var templates: [Template] = []
    private let timer = Timer.publish(every: 3, on: .main, in: .common).autoconnect()

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text(model.status).font(.callout).foregroundColor(.secondary)
                Spacer()
                if model.busy { ProgressView().controlSize(.small) }
                if !model.engineDown && !model.boxes.isEmpty {
                    Button(action: { showCreate = true }) { Label("Novo", systemImage: "plus") }
                        .disabled(model.busy)
                }
                Button(action: { model.refresh() }) { Image(systemName: "arrow.clockwise") }
                    .disabled(model.busy)
            }

            if let f = model.flash {
                HStack(spacing: 6) {
                    Image(systemName: f.isError ? "exclamationmark.triangle.fill" : "checkmark.circle.fill")
                        .foregroundColor(f.isError ? .orange : .green)
                    Text(f.text).font(.caption).foregroundColor(f.isError ? .orange : .secondary)
                        .lineLimit(2).textSelection(.enabled)
                    Spacer()
                    Button(action: { model.flash = nil }) { Image(systemName: "xmark") }
                        .buttonStyle(.borderless)
                }
                .padding(8)
                .background(Color(nsColor: .textBackgroundColor))
                .cornerRadius(6)
            }

            if model.engineDown {
                Spacer()
                VStack(spacing: 12) {
                    Image(systemName: "bolt.horizontal.circle")
                        .font(.system(size: 42)).foregroundColor(.orange)
                    Text("O OrbStack não está rodando").font(.headline)
                    Text("O Peapod precisa do OrbStack (ou Docker) para criar sandboxes.")
                        .foregroundColor(.secondary).multilineTextAlignment(.center)
                    HStack {
                        Button("Abrir OrbStack") { model.openEngine() }
                            .buttonStyle(.borderedProminent)
                        Button("Tentar de novo") { model.refresh() }
                    }
                }
                .frame(maxWidth: .infinity, alignment: .center)
                Spacer()
            } else if model.boxes.isEmpty {
                CreatePanel(model: model, templates: templates)
            } else {
                List(model.boxes) { b in
                    HStack(spacing: 8) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(b.id).font(.system(.body, design: .monospaced))
                            Text("\(b.image) · \(b.network)").font(.caption).foregroundColor(.secondary)
                        }
                        if b.paused == true {
                            Text("pausado").font(.caption).foregroundColor(.orange)
                        }
                        Spacer()
                        Button("Logs") { selected = b }
                        Button("Snapshot") { model.snapshot(b.id) }
                        if b.paused == true {
                            Button("Retomar") { model.resume(b.id) }
                        } else {
                            Button("Pausar") { model.pause(b.id) }
                        }
                        Button(role: .destructive) { model.destroy(b.id) } label: {
                            Image(systemName: "trash")
                        }
                    }
                    .buttonStyle(.borderless)
                    .padding(.vertical, 2)
                    .disabled(model.busy)
                }
            }
        }
        .padding(16)
        .frame(minWidth: 620, minHeight: 440)
        .onAppear {
            model.refresh()
            DispatchQueue.global(qos: .utility).async {
                let t = fetchTemplates()
                DispatchQueue.main.async { templates = t }
            }
        }
        .onReceive(timer) { _ in if !model.busy { model.refresh() } }
        .sheet(item: $selected) { b in DetailView(box: b) }
        .sheet(isPresented: $showCreate) {
            VStack(alignment: .leading, spacing: 12) {
                HStack {
                    Text("Novo sandbox").font(.headline)
                    Spacer()
                    Button("Fechar") { showCreate = false }
                }
                CreatePanel(model: model, templates: templates) { showCreate = false }
            }
            .padding(16)
            .frame(width: 520, height: 460)
        }
    }
}

// --- Renderizador de Markdown mínimo (cabeçalhos, listas, código, citações) ---

enum MDBlock {
    case heading(Int, String)
    case paragraph(String)
    case bullet(String)
    case quote(String)
    case code(String)
    case image(String)
    case divider
}

func parseMarkdown(_ md: String) -> [MDBlock] {
    var blocks: [MDBlock] = []
    var code: [String]? = nil
    for raw in md.components(separatedBy: "\n") {
        let t = raw.trimmingCharacters(in: .whitespaces)
        if t.hasPrefix("```") {
            if code == nil { code = [] } else { blocks.append(.code(code!.joined(separator: "\n"))); code = nil }
            continue
        }
        if code != nil { code!.append(raw); continue }
        if t.isEmpty { continue }
        if t == "---" || t == "***" { blocks.append(.divider); continue }
        if t.hasPrefix("#### ") { blocks.append(.heading(4, String(t.dropFirst(5)))); continue }
        if t.hasPrefix("### ") { blocks.append(.heading(3, String(t.dropFirst(4)))); continue }
        if t.hasPrefix("## ") { blocks.append(.heading(2, String(t.dropFirst(3)))); continue }
        if t.hasPrefix("# ") { blocks.append(.heading(1, String(t.dropFirst(2)))); continue }
        if t.hasPrefix("- ") || t.hasPrefix("* ") { blocks.append(.bullet(String(t.dropFirst(2)))); continue }
        if t.hasPrefix("> ") { blocks.append(.quote(String(t.dropFirst(2)))); continue }
        if t.hasPrefix("!["), let r = t.range(of: "]("), t.hasSuffix(")") {
            blocks.append(.image(String(t[r.upperBound...].dropLast()))); continue
        }
        blocks.append(.paragraph(t))
    }
    if let c = code { blocks.append(.code(c.joined(separator: "\n"))) }
    return blocks
}

func mdInline(_ s: String) -> AttributedString {
    (try? AttributedString(markdown: s, options: .init(interpretedSyntax: .inlineOnlyPreservingWhitespace)))
        ?? AttributedString(s)
}

func mdImageURL(_ src: String) -> URL? {
    let ns = src as NSString
    return Bundle.main.url(forResource: ns.deletingPathExtension, withExtension: ns.pathExtension)
}

struct MarkdownView: View {
    let markdown: String
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 8) {
                ForEach(Array(parseMarkdown(markdown).enumerated()), id: \.offset) { _, b in
                    block(b)
                }
            }
            .textSelection(.enabled)
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    @ViewBuilder
    private func block(_ b: MDBlock) -> some View {
        switch b {
        case .heading(let lvl, let txt):
            Text(mdInline(txt)).font(headingFont(lvl)).bold().padding(.top, lvl <= 2 ? 10 : 4)
        case .paragraph(let txt):
            Text(mdInline(txt))
        case .bullet(let txt):
            HStack(alignment: .top, spacing: 8) { Text("•"); Text(mdInline(txt)) }
        case .quote(let txt):
            Text(mdInline(txt)).foregroundColor(.secondary).padding(.leading, 12)
                .overlay(Rectangle().fill(Color.secondary.opacity(0.4)).frame(width: 3), alignment: .leading)
        case .code(let txt):
            ScrollView(.horizontal, showsIndicators: false) {
                Text(txt).font(.system(.callout, design: .monospaced)).padding(10)
            }
            .background(Color(nsColor: .textBackgroundColor)).cornerRadius(6)
        case .image(let src):
            if let url = mdImageURL(src), let img = NSImage(contentsOf: url) {
                Image(nsImage: img).resizable().scaledToFit().frame(maxWidth: 720).padding(.vertical, 4)
            } else {
                Text("[imagem: \(src)]").font(.caption).foregroundColor(.secondary)
            }
        case .divider:
            Divider().padding(.vertical, 4)
        }
    }

    private func headingFont(_ lvl: Int) -> Font {
        switch lvl {
        case 1: return .title
        case 2: return .title2
        case 3: return .title3
        default: return .headline
        }
    }
}

struct ManualView: View {
    @State private var tab = 0 // 0 = Para todos, 1 = Técnico
    var body: some View {
        VStack(spacing: 0) {
            Picker("", selection: $tab) {
                Text("Para todos").tag(0)
                Text("Técnico").tag(1)
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .padding(10)
            Divider()
            MarkdownView(markdown: Self.load(tab == 0 ? "GUIA" : "MANUAL"))
        }
        .frame(minWidth: 660, minHeight: 560)
    }
    static func load(_ name: String) -> String {
        if let url = Bundle.main.url(forResource: name, withExtension: "md"),
           let s = try? String(contentsOf: url, encoding: .utf8) {
            return s
        }
        return "# \(name)\n\nNão foi possível carregar."
    }
}

struct HelpCommands: Commands {
    @Environment(\.openWindow) private var openWindow
    var body: some Commands {
        CommandGroup(replacing: .help) {
            Button("Manual do Peapod") { openWindow(id: "manual") }
                .keyboardShortcut("?", modifiers: [.command])
        }
    }
}

@main
struct PeapodApp: App {
    init() {
        // A interface do app é em português (pt-BR). Por padrão os menus padrão
        // do macOS também são forçados para pt-BR; quem preferir os menus no
        // idioma do sistema desliga isso uma vez com:
        //   defaults write dev.peapod.ui PeapodFollowSystemLanguage -bool true
        if UserDefaults.standard.bool(forKey: "PeapodFollowSystemLanguage") {
            UserDefaults.standard.removeObject(forKey: "AppleLanguages")
        } else {
            UserDefaults.standard.set(["pt-BR"], forKey: "AppleLanguages")
        }
        // App utilitário de janela única — remove o "+" de novas abas no título.
        NSWindow.allowsAutomaticWindowTabbing = false
    }
    var body: some Scene {
        WindowGroup("Peapod") {
            ContentView()
        }
        .defaultSize(width: 660, height: 500)
        .commands { HelpCommands() }

        Window("Manual do Peapod", id: "manual") {
            ManualView()
        }
        .defaultSize(width: 800, height: 720)
    }
}
