import Cocoa
import Sparkle

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
    var controlClient: ControlClient?
    var loginItem: NSMenuItem!
    var preferencesWindow: PreferencesWindowController?
    var logsWindow: LogsWindowController?
    var startWorkerItem: NSMenuItem!
    var stopWorkerItem: NSMenuItem!
    var drainItem: NSMenuItem!
    var undrainItem: NSMenuItem!
    var shutdownItem: NSMenuItem!
    let updaterController = SPUStandardUpdaterController(startingUpdater: true, updaterDelegate: nil, userDriverDelegate: nil)

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
        startWorkerItem = NSMenuItem(title: "Start Worker", action: #selector(startWorker), keyEquivalent: "")
        menu.addItem(startWorkerItem)
        stopWorkerItem = NSMenuItem(title: "Stop Worker", action: #selector(stopWorker), keyEquivalent: "")
        menu.addItem(stopWorkerItem)
        drainItem = NSMenuItem(title: "Drain (finish current jobs, stop new)", action: #selector(drainWorker), keyEquivalent: "")
        drainItem.isEnabled = false
        menu.addItem(drainItem)
        undrainItem = NSMenuItem(title: "Undrain", action: #selector(undrainWorker), keyEquivalent: "")
        undrainItem.isEnabled = false
        menu.addItem(undrainItem)
        shutdownItem = NSMenuItem(title: "Shutdown after drain", action: #selector(shutdownAfterDrain), keyEquivalent: "")
        shutdownItem.isEnabled = false
        menu.addItem(shutdownItem)
        menu.addItem(NSMenuItem.separator())
        menu.addItem(NSMenuItem(title: "Preferences…", action: #selector(openPreferences), keyEquivalent: ","))
        menu.addItem(NSMenuItem(title: "Logs…", action: #selector(openLogs), keyEquivalent: ""))
        menu.addItem(NSMenuItem(title: "Copy Diagnostics", action: #selector(copyDiagnostics), keyEquivalent: ""))
        menu.addItem(NSMenuItem(title: "Open Config Folder", action: #selector(openConfigFolder), keyEquivalent: ""))
        menu.addItem(NSMenuItem(title: "Open Logs Folder", action: #selector(openLogsFolder), keyEquivalent: ""))
        loginItem = NSMenuItem(title: "Start at Login", action: #selector(toggleStartAtLogin), keyEquivalent: "")
        loginItem.state = LaunchAgentManager.shared.isRunAtLoadEnabled() ? .on : .off
        menu.addItem(loginItem)
        menu.addItem(NSMenuItem(title: "Check for Updates", action: #selector(checkForUpdates), keyEquivalent: ""))
        menu.addItem(NSMenuItem.separator())
        menu.addItem(NSMenuItem(title: "Quit", action: #selector(quit), keyEquivalent: "q"))
        statusItem.menu = menu

        updaterController.updater.checkForUpdatesInBackground()

        statusClient = StatusClient()
        statusClient?.onUpdate = { [weak self] result in
            DispatchQueue.main.async {
                self?.handleStatusResult(result)
            }
        }
        statusClient?.start()
        controlClient = ControlClient()
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
            startWorkerItem.isEnabled = status.state == .disconnected
            stopWorkerItem.isEnabled = status.state != .disconnected
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
            startWorkerItem.isEnabled = true
            stopWorkerItem.isEnabled = false
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
        do {
            try LaunchAgentManager.shared.start()
        } catch {
            print("Failed to start worker: \(error)")
        }
    }

    @objc func stopWorker(_ sender: Any?) {
        do {
            try LaunchAgentManager.shared.stop()
        } catch {
            print("Failed to stop worker: \(error)")
        }
    }

    @objc func drainWorker(_ sender: Any?) {
        controlClient?.drain()
    }

    @objc func undrainWorker(_ sender: Any?) {
        controlClient?.undrain()
    }

    @objc func shutdownAfterDrain(_ sender: Any?) {
        controlClient?.terminateAfterDrain()
    }

    @objc func openPreferences(_ sender: Any?) {
        if preferencesWindow == nil {
            preferencesWindow = PreferencesWindowController()
        }
        preferencesWindow?.showWindow(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    @objc func openConfigFolder(_ sender: Any?) {
        ConfigManager.shared.openConfigFolder()
    }

    @objc func openLogsFolder(_ sender: Any?) {
        ConfigManager.shared.openLogsFolder()
    }

    @objc func openLogs(_ sender: Any?) {
        if logsWindow == nil {
            logsWindow = LogsWindowController()
        }
        logsWindow?.showWindow(nil)
        NSApp.activate(ignoringOtherApps: true)
    }

    @objc func copyDiagnostics(_ sender: Any?) {
        do {
            let url = try ConfigManager.shared.copyDiagnostics()
            let alert = NSAlert()
            alert.messageText = "Diagnostics copied to Desktop"
            alert.informativeText = url.path
            alert.runModal()
        } catch {
            let alert = NSAlert(error: error)
            alert.runModal()
        }
    }

    @objc func toggleStartAtLogin(_ sender: NSMenuItem) {
        let enable = sender.state == .off
        do {
            try LaunchAgentManager.shared.setRunAtLoad(enable)
            sender.state = enable ? .on : .off
        } catch {
            print("Failed to toggle Start at Login: \(error)")
        }
    }

    @objc func checkForUpdates(_ sender: Any?) {
        updaterController.checkForUpdates(sender)
    }

    @objc func quit(_ sender: Any?) {
        NSApplication.shared.terminate(nil)
    }
}
