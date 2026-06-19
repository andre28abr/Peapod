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

// peapodBin finds the peapod CLI: bundled first, then overrides, then PATH.
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

// childEnv augments PATH so the bundled peapod can find docker/orbstack, which
// live in /usr/local/bin — not on a GUI app's default PATH.
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

    func create() { _ = runPeapod(["sandbox", "create", "alpine"]); refresh() }
    func destroy(_ id: String) { _ = runPeapod(["sandbox", "rm", id]); refresh() }
    func pause(_ id: String) { _ = runPeapod(["sandbox", "pause", id]); refresh() }
    func resume(_ id: String) { _ = runPeapod(["sandbox", "resume", id]); refresh() }
}

struct ContentView: View {
    @StateObject var model = Model()
    let timer = Timer.publish(every: 3, on: .main, in: .common).autoconnect()

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Text("Peapod").font(.title2).bold()
                Spacer()
                Button(action: { model.create() }) { Label("New alpine", systemImage: "plus") }
                Button(action: { model.refresh() }) { Image(systemName: "arrow.clockwise") }
            }
            Text(model.status).font(.caption).foregroundColor(.secondary)
            if model.boxes.isEmpty {
                Spacer()
                Text("No sandboxes.\nClick “New alpine” to create one.")
                    .multilineTextAlignment(.center)
                    .foregroundColor(.secondary)
                    .frame(maxWidth: .infinity, alignment: .center)
                Spacer()
            } else {
                List(model.boxes) { b in
                    HStack(spacing: 10) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(b.id).font(.system(.body, design: .monospaced))
                            Text("\(b.image) · \(b.network)").font(.caption).foregroundColor(.secondary)
                        }
                        if b.paused == true {
                            Text("paused").font(.caption).foregroundColor(.orange)
                        }
                        Spacer()
                        if b.paused == true {
                            Button("Resume") { model.resume(b.id) }
                        } else {
                            Button("Pause") { model.pause(b.id) }
                        }
                        Button(role: .destructive) { model.destroy(b.id) } label: {
                            Image(systemName: "trash")
                        }
                    }
                    .padding(.vertical, 2)
                }
            }
        }
        .padding(16)
        .frame(minWidth: 480, minHeight: 380)
        .onAppear { model.refresh() }
        .onReceive(timer) { _ in model.refresh() }
    }
}

@main
struct PeapodApp: App {
    var body: some Scene {
        WindowGroup("Peapod") {
            ContentView()
        }
        .defaultSize(width: 520, height: 440)
    }
}
