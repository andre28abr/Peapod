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

@MainActor
final class Model: ObservableObject {
    @Published var boxes: [Sandbox] = []
    @Published var status: String = "loading…"

    func refresh() {
        let r = runPeapod(["sandbox", "ls", "--json"])
        guard r.ok, let data = r.out.data(using: .utf8),
              let list = try? JSONDecoder().decode([Sandbox].self, from: data) else {
            boxes = []
            let e = r.err.trimmingCharacters(in: .whitespacesAndNewlines)
            status = e.isEmpty ? "can't reach peapod — is OrbStack/Docker running?" : e
            return
        }
        boxes = list
        status = list.isEmpty ? "no sandboxes yet" : "\(list.count) sandbox(es)"
    }

    func create(_ image: String) {
        let img = image.trimmingCharacters(in: .whitespaces)
        _ = runPeapod(["sandbox", "create", img.isEmpty ? "alpine" : img])
        refresh()
    }
    func destroy(_ id: String) { _ = runPeapod(["sandbox", "rm", id]); refresh() }
    func pause(_ id: String) { _ = runPeapod(["sandbox", "pause", id]); refresh() }
    func resume(_ id: String) { _ = runPeapod(["sandbox", "resume", id]); refresh() }

    func snapshot(_ id: String) {
        let name = "\(id)-\(Int(Date().timeIntervalSince1970))"
        let r = runPeapod(["sandbox", "snapshot", id, name])
        status = r.ok ? "snapshot: " + r.out.trimmingCharacters(in: .whitespacesAndNewlines)
                      : "snapshot failed"
    }

    func logs(_ id: String) -> String {
        let r = runPeapod(["sandbox", "logs", id, "--tail", "200"])
        let t = (r.out + r.err).trimmingCharacters(in: .whitespacesAndNewlines)
        return t.isEmpty ? "(no output yet)" : t
    }

    func stats(_ id: String) -> Stat? {
        let r = runPeapod(["sandbox", "stats", id, "--json"])
        guard r.ok, let d = r.out.data(using: .utf8) else { return nil }
        return try? JSONDecoder().decode(Stat.self, from: d)
    }
}

struct DetailView: View {
    let model: Model
    let box: Sandbox
    @Environment(\.dismiss) private var dismiss
    @State private var logs = "loading…"
    @State private var stat: Stat?

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text(box.id).font(.system(.title3, design: .monospaced))
                Spacer()
                Button("Refresh") { load() }
                Button("Close") { dismiss() }
            }
            Text("\(box.image) · \(box.network)").font(.caption).foregroundColor(.secondary)
            if let s = stat {
                HStack(spacing: 18) {
                    Label(s.cpu_perc, systemImage: "cpu")
                    Label("\(s.mem_usage) (\(s.mem_perc))", systemImage: "memorychip")
                }.font(.caption)
            }
            Text("Logs").font(.headline)
            ScrollView {
                Text(logs)
                    .font(.system(.caption, design: .monospaced))
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(8)
            }
            .frame(minHeight: 220)
            .background(Color(nsColor: .textBackgroundColor))
            .cornerRadius(6)
        }
        .padding(16)
        .frame(width: 540, height: 400)
        .onAppear { load() }
    }

    private func load() {
        stat = model.stats(box.id)
        logs = model.logs(box.id)
    }
}

struct ContentView: View {
    @StateObject private var model = Model()
    @State private var image = "alpine"
    @State private var selected: Sandbox?
    private let timer = Timer.publish(every: 3, on: .main, in: .common).autoconnect()

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Peapod").font(.title2).bold()
                Spacer()
                TextField("image", text: $image)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 160)
                    .onSubmit { model.create(image) }
                Button(action: { model.create(image) }) { Label("New", systemImage: "plus") }
                Button(action: { model.refresh() }) { Image(systemName: "arrow.clockwise") }
            }
            Text(model.status).font(.caption).foregroundColor(.secondary)

            if model.boxes.isEmpty {
                Spacer()
                Text("No sandboxes.\nType an image and click New.")
                    .multilineTextAlignment(.center)
                    .foregroundColor(.secondary)
                    .frame(maxWidth: .infinity, alignment: .center)
                Spacer()
            } else {
                List(model.boxes) { b in
                    HStack(spacing: 8) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(b.id).font(.system(.body, design: .monospaced))
                            Text("\(b.image) · \(b.network)").font(.caption).foregroundColor(.secondary)
                        }
                        if b.paused == true {
                            Text("paused").font(.caption).foregroundColor(.orange)
                        }
                        Spacer()
                        Button("Logs") { selected = b }
                        Button("Snapshot") { model.snapshot(b.id) }
                        if b.paused == true {
                            Button("Resume") { model.resume(b.id) }
                        } else {
                            Button("Pause") { model.pause(b.id) }
                        }
                        Button(role: .destructive) { model.destroy(b.id) } label: {
                            Image(systemName: "trash")
                        }
                    }
                    .buttonStyle(.borderless)
                    .padding(.vertical, 2)
                }
            }
        }
        .padding(16)
        .frame(minWidth: 620, minHeight: 440)
        .onAppear { model.refresh() }
        .onReceive(timer) { _ in model.refresh() }
        .sheet(item: $selected) { b in DetailView(model: model, box: b) }
    }
}

@main
struct PeapodApp: App {
    var body: some Scene {
        WindowGroup("Peapod") {
            ContentView()
        }
        .defaultSize(width: 660, height: 500)
    }
}
