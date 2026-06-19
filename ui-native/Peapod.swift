// Peapod — minimal native macOS menu-bar app (SwiftUI).
// Talks to the peapod CLI (`peapod sandbox ls --json`, create, rm).
// Set PEAPOD_BIN to the peapod binary if it isn't at /usr/local/bin/peapod.
import SwiftUI
import AppKit

struct Sandbox: Codable, Identifiable {
    let id: String
    let image: String
    let network: String
    let name: String?
    let paused: Bool?
}

func peapodBin() -> String {
    if let b = ProcessInfo.processInfo.environment["PEAPOD_BIN"], !b.isEmpty { return b }
    return "/usr/local/bin/peapod"
}

@discardableResult
func runPeapod(_ args: [String]) -> (out: String, ok: Bool) {
    let p = Process()
    p.executableURL = URL(fileURLWithPath: peapodBin())
    p.arguments = args
    let pipe = Pipe()
    p.standardOutput = pipe
    p.standardError = Pipe()
    do { try p.run() } catch { return ("", false) }
    let data = pipe.fileHandleForReading.readDataToEndOfFile()
    p.waitUntilExit()
    return (String(data: data, encoding: .utf8) ?? "", p.terminationStatus == 0)
}

@MainActor
final class Model: ObservableObject {
    @Published var boxes: [Sandbox] = []
    @Published var error: String = ""

    func refresh() {
        let r = runPeapod(["sandbox", "ls", "--json"])
        guard r.ok, let data = r.out.data(using: .utf8) else {
            error = "cannot reach peapod (set PEAPOD_BIN?)"
            return
        }
        do {
            boxes = try JSONDecoder().decode([Sandbox].self, from: data)
            error = ""
        } catch {
            self.error = "decode error"
        }
    }

    func create() { _ = runPeapod(["sandbox", "create", "alpine", "--name", "ui"]); refresh() }
    func destroy(_ id: String) { _ = runPeapod(["sandbox", "rm", id]); refresh() }
}

@main
struct PeapodApp: App {
    @StateObject private var model = Model()

    var body: some Scene {
        MenuBarExtra("Peapod", systemImage: "shippingbox") {
            VStack(alignment: .leading, spacing: 6) {
                Text("Peapod sandboxes").font(.headline)
                if !model.error.isEmpty {
                    Text(model.error).foregroundColor(.red).font(.caption)
                }
                if model.boxes.isEmpty {
                    Text("no sandboxes").foregroundColor(.secondary).font(.caption)
                }
                ForEach(model.boxes) { b in
                    HStack {
                        Text(b.id).font(.system(.body, design: .monospaced))
                        Text(b.image).foregroundColor(.secondary)
                        if b.paused == true {
                            Text("paused").foregroundColor(.orange).font(.caption)
                        }
                        Spacer()
                        Button("✕") { model.destroy(b.id) }
                    }
                }
                Divider()
                Button("Create alpine") { model.create() }
                Button("Refresh") { model.refresh() }
                Button("Quit") { NSApplication.shared.terminate(nil) }
            }
            .padding(10)
            .frame(width: 340)
            .onAppear { model.refresh() }
        }
        .menuBarExtraStyle(.window)
    }
}
