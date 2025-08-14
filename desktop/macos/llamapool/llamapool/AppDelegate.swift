import Cocoa

@main
class AppDelegate: NSObject, NSApplicationDelegate {
    var statusItem: NSStatusItem!
    var statusMenuItem: NSMenuItem!
    var workerItem: NSMenuItem!
    var versionItem: NSMenuItem!
    var connectionItem: NSMenuItem!
    var jobsItem: NSMenuItem!
    var lastErrorItem: NSMenuItem!
    var statusClient: StatusClient?

    func applicationDidFinishLaunching(_ notification: Notification) {
        statusItem = NSStatusBar.system.statusItem(withLength: NSStatusItem.variableLength)
        updateStatusDot(color: .systemRed)

        let menu = NSMenu()
        statusMenuItem = NSMenuItem(title: "Status: Disconnected", action: nil, keyEquivalent: "")
        statusMenuItem.isEnabled = false
        menu.addItem(statusMenuItem)

        let detailsItem = NSMenuItem(title: "Details…", action: nil, keyEquivalent: "")
        let detailsMenu = NSMenu()
        workerItem = NSMenuItem(title: "Worker: –", action: nil, keyEquivalent: "")
        versionItem = NSMenuItem(title: "Version: –", action: nil, keyEquivalent: "")
        connectionItem = NSMenuItem(title: "Connected: Server –, Ollama –", action: nil, keyEquivalent: "")
        jobsItem = NSMenuItem(title: "Jobs: 0 / 0", action: nil, keyEquivalent: "")
        lastErrorItem = NSMenuItem(title: "Last Error: None", action: nil, keyEquivalent: "")
        detailsMenu.addItem(workerItem)
        detailsMenu.addItem(versionItem)
        detailsMenu.addItem(connectionItem)
        detailsMenu.addItem(jobsItem)
        detailsMenu.addItem(lastErrorItem)
        detailsItem.submenu = detailsMenu
        menu.addItem(detailsItem)

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

        statusClient = StatusClient()
        statusClient?.onUpdate = { [weak self] result in
            DispatchQueue.main.async {
                self?.handleStatusResult(result)
            }
        }
        statusClient?.start()
    }

    func updateStatusDot(color: NSColor) {
        let dot = NSAttributedString(string: "●", attributes: [.foregroundColor: color])
        statusItem.button?.attributedTitle = dot
    }

    func handleStatusResult(_ result: Result<WorkerStatus, Error>) {
        switch result {
        case .success(let status):
            statusItem.button?.toolTip = nil
            statusMenuItem.toolTip = nil
            statusMenuItem.title = "Status: \(text(for: status.state))"
            updateStatusDot(color: color(for: status.state))
            workerItem.title = "Worker: \(status.workerName) (\(status.workerId))"
            versionItem.title = "Version: \(status.version)"
            connectionItem.title = "Connected: Server \(status.connectedToServer ? "Yes" : "No"), Ollama \(status.connectedToOllama ? "Yes" : "No")"
            jobsItem.title = "Jobs: \(status.currentJobs) / \(status.maxConcurrency)"
            lastErrorItem.title = status.lastError.isEmpty ? "Last Error: None" : "Last Error: \(status.lastError)"
        case .failure(let error):
            statusMenuItem.title = "Status: Disconnected"
            statusItem.button?.toolTip = error.localizedDescription
            statusMenuItem.toolTip = error.localizedDescription
            updateStatusDot(color: .systemRed)
            workerItem.title = "Worker: –"
            versionItem.title = "Version: –"
            connectionItem.title = "Connected: Server –, Ollama –"
            jobsItem.title = "Jobs: 0 / 0"
            lastErrorItem.title = "Last Error: \(error.localizedDescription)"
        }
    }

    func text(for state: WorkerStatus.State) -> String {
        switch state {
        case .connectedIdle: return "Connected Idle"
        case .connectedBusy: return "Connected Busy"
        case .connecting: return "Connecting"
        case .disconnected: return "Disconnected"
        case .draining: return "Draining"
        case .terminating: return "Terminating"
        case .error: return "Error"
        }
    }

    func color(for state: WorkerStatus.State) -> NSColor {
        switch state {
        case .connectedIdle, .connectedBusy:
            return .systemGreen
        case .connecting, .draining, .terminating:
            return .systemYellow
        case .disconnected, .error:
            return .systemRed
        }
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
