import Cocoa
import WebKit

final class AppDelegate: NSObject, NSApplicationDelegate {
    var window: NSWindow!
    var webview: WKWebView!
    var proc: Process?

    func applicationDidFinishLaunching(_ note: Notification) {
        let bin = Bundle.main.resourcePath! + "/skillsync"
        let p = Process()
        p.executableURL = URL(fileURLWithPath: bin)
        p.arguments = ["ui", "--port", "0", "--no-browser"]
        let pipe = Pipe()
        p.standardOutput = pipe
        p.standardError = FileHandle.nullDevice
        do { try p.run() } catch { fail("无法启动同步服务") ; return }
        proc = p

        window = NSWindow(
            contentRect: NSRect(x: 0, y: 0, width: 1280, height: 800),
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered, defer: false)
        window.title = "skillsync"
        webview = WKWebView(frame: window.contentView!.bounds)
        webview.autoresizingMask = [.width, .height]
        window.contentView!.addSubview(webview)
        window.center()
        window.makeKeyAndOrderFront(nil)
        NSApp.activate(ignoringOtherApps: true)

        DispatchQueue.global().async {
            let h = pipe.fileHandleForReading
            var buf = Data()
            while true {
                let d = h.availableData
                if d.isEmpty { break }
                buf.append(d)
                if let s = String(data: buf, encoding: .utf8),
                   let r = s.range(of: "LISTENING ") {
                    let rest = s[r.upperBound...].components(separatedBy: .whitespacesAndNewlines)[0]
                    if let port = Int(rest) {
                        DispatchQueue.main.async {
                            self.webview.load(URLRequest(url: URL(string: "http://127.0.0.1:\(port)/")!))
                        }
                        return
                    }
                }
            }
            DispatchQueue.main.async { self.fail("同步服务未能启动") }
        }
    }

    func fail(_ msg: String) {
        let a = NSAlert(); a.messageText = "skillsync"; a.informativeText = msg
        a.runModal()
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ app: NSApplication) -> Bool { true }

    func applicationWillTerminate(_ note: Notification) { proc?.terminate() }
}

let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.setActivationPolicy(.regular)

let mainMenu = NSMenu()
let appItem = NSMenuItem()
mainMenu.addItem(appItem)
let appMenu = NSMenu()
appMenu.addItem(withTitle: "关于 skillsync", action: #selector(NSApplication.orderFrontStandardAboutPanel(_:)), keyEquivalent: "")
appMenu.addItem(.separator())
appMenu.addItem(withTitle: "隐藏 skillsync", action: #selector(NSApplication.hide(_:)), keyEquivalent: "h")
appMenu.addItem(.separator())
appMenu.addItem(withTitle: "退出 skillsync", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
appItem.submenu = appMenu
app.mainMenu = mainMenu

app.run()
