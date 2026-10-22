import Cocoa
import WebKit

final class AppDelegate: NSObject, NSApplicationDelegate {
    var window: NSWindow!
    var webview: WKWebView!
    var proc: Process?
    let port = "7788"

    func applicationDidFinishLaunching(_ note: Notification) {
        let bin = Bundle.main.resourcePath! + "/skillsync"
        let p = Process()
        p.executableURL = URL(fileURLWithPath: bin)
        p.arguments = ["ui", "--port", port, "--no-browser"]
        try? p.run()
        proc = p

        window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 1280, height: 800),
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered, defer: false)
        window.title = "skillsync"
        webview = WKWebView(frame: window.contentView!.bounds)
        webview.autoresizingMask = [.width, .height]
        window.contentView!.addSubview(webview)
        pollAndLoad()
        window.center()
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    func pollAndLoad() {
        let base = "http://127.0.0.1:\(port)"
        URLSession.shared.dataTask(with: URL(string: base + "/api/status")!) { _, resp, err in
            if err == nil, (resp as? HTTPURLResponse)?.statusCode == 200 {
                DispatchQueue.main.async {
                    self.webview.load(URLRequest(url: URL(string: base + "/")!))
                }
            } else {
                DispatchQueue.main.asyncAfter(deadline: .now() + 0.3) { self.pollAndLoad() }
            }
        }.resume()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ app: NSApplication) -> Bool { true }

    func applicationWillTerminate(_ note: Notification) { proc?.terminate() }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.regular)
app.run()
