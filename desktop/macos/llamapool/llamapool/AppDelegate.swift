import Cocoa

@main
class AppDelegate: NSObject, NSApplicationDelegate {
    var statusItem: NSStatusItem!

    func applicationDidFinishLaunching(_ notification: Notification) {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        if let button = statusItem.button {
            button.image = NSImage(named: "AppIcon")
        }

        let menu = NSMenu()
        let statusMenuItem = NSMenuItem(title: "Status: Disconnected", action: nil, keyEquivalent: "")
        statusMenuItem.isEnabled = false
        menu.addItem(statusMenuItem)
        menu.addItem(NSMenuItem.separator())
        menu.addItem(NSMenuItem(title: "Start Worker", action: #selector(startWorker), keyEquivalent: ""))
        menu.addItem(NSMenuItem(title: "Stop Worker", action: #selector(stopWorker), keyEquivalent: ""))
        menu.addItem(NSMenuItem.separator())
        menu.addItem(NSMenuItem(title: "Preferences…", action: #selector(openPreferences), keyEquivalent: ","))
        menu.addItem(NSMenuItem(title: "Logs…", action: #selector(openLogs), keyEquivalent: "l"))
        let loginItem = NSMenuItem(title: "Start at Login", action: #selector(toggleStartAtLogin), keyEquivalent: "")
        loginItem.state = .off
        menu.addItem(loginItem)
        menu.addItem(NSMenuItem(title: "Check for Updates", action: #selector(checkForUpdates), keyEquivalent: ""))
        menu.addItem(NSMenuItem.separator())
        menu.addItem(NSMenuItem(title: "Quit", action: #selector(quit), keyEquivalent: "q"))
        statusItem.menu = menu
    }

    @objc func startWorker(_ sender: Any?) {
        print("Start Worker clicked")
    }

    @objc func stopWorker(_ sender: Any?) {
        print("Stop Worker clicked")
    }

    @objc func openPreferences(_ sender: Any?) {
        print("Preferences clicked")
    }

    @objc func openLogs(_ sender: Any?) {
        print("Logs clicked")
    }

    @objc func toggleStartAtLogin(_ sender: NSMenuItem) {
        sender.state = sender.state == .on ? .off : .on
        print("Start at Login toggled")
    }

    @objc func checkForUpdates(_ sender: Any?) {
        print("Check for Updates clicked")
    }

    @objc func quit(_ sender: Any?) {
        NSApplication.shared.terminate(nil)
    }
}
