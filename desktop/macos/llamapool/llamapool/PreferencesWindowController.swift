import Cocoa

class PreferencesWindowController: NSWindowController {
    private let form = NSForm(frame: NSRect(x: 20, y: 60, width: 360, height: 140))
    private var serverEntry: NSFormCell!
    private var keyEntry: NSFormCell!
    private var ollamaEntry: NSFormCell!
    private var concurrencyEntry: NSFormCell!
    private var portEntry: NSFormCell!

    init() {
        let window = NSWindow(contentRect: NSRect(x: 0, y: 0, width: 400, height: 250),
                              styleMask: [.titled, .closable],
                              backing: .buffered, defer: false)
        window.center()
        window.title = "Preferences"
        super.init(window: window)

        serverEntry = form.addEntry("Server URL:")
        keyEntry = form.addEntry("Worker Key:")
        ollamaEntry = form.addEntry("Ollama Base URL:")
        concurrencyEntry = form.addEntry("Max Concurrency:")
        portEntry = form.addEntry("Status Port:")
        window.contentView?.addSubview(form)

        let saveButton = NSButton(title: "Save", target: self, action: #selector(save))
        saveButton.frame = NSRect(x: 220, y: 20, width: 80, height: 30)
        let cancelButton = NSButton(title: "Cancel", target: self, action: #selector(cancel))
        cancelButton.frame = NSRect(x: 310, y: 20, width: 80, height: 30)
        window.contentView?.addSubview(saveButton)
        window.contentView?.addSubview(cancelButton)

        let config = ConfigManager.shared.load()
        serverEntry.stringValue = config.serverURL
        keyEntry.stringValue = config.workerKey
        ollamaEntry.stringValue = config.ollamaBaseURL
        concurrencyEntry.stringValue = String(config.maxConcurrency)
        portEntry.stringValue = String(config.statusPort)
    }

    required init?(coder: NSCoder) {
        fatalError("init(coder:) has not been implemented")
    }

    @objc private func save() {
        guard !serverEntry.stringValue.isEmpty,
              let maxConc = Int(concurrencyEntry.stringValue),
              let port = Int(portEntry.stringValue) else {
            let alert = NSAlert()
            alert.messageText = "Invalid input"
            alert.runModal()
            return
        }
        let config = WorkerConfig(serverURL: serverEntry.stringValue,
                                  workerKey: keyEntry.stringValue,
                                  ollamaBaseURL: ollamaEntry.stringValue,
                                  maxConcurrency: maxConc,
                                  statusPort: port)
        guard config.isValid() else {
            let alert = NSAlert()
            alert.messageText = "Invalid configuration"
            alert.runModal()
            return
        }
        do {
            try ConfigManager.shared.save(config)
        } catch {
            let alert = NSAlert(error: error)
            alert.runModal()
            return
        }
        if LaunchAgentManager.shared.isAgentLoaded() {
            let alert = NSAlert()
            alert.messageText = "Restart worker to apply changes?"
            alert.addButton(withTitle: "Restart")
            alert.addButton(withTitle: "Later")
            let response = alert.runModal()
            if response == .alertFirstButtonReturn {
                try? LaunchAgentManager.shared.stop()
                try? LaunchAgentManager.shared.start()
            }
        }
        window?.close()
    }

    @objc private func cancel() {
        window?.close()
    }
}
