import Cocoa

class ConfigManager {
    static let shared = ConfigManager()
    private let fileManager = FileManager.default
    private init() {}

    var configDirURL: URL {
        fileManager.homeDirectoryForCurrentUser.appendingPathComponent("Library/Application Support/nfrx", isDirectory: true)
    }

    var configFileURL: URL {
        configDirURL.appendingPathComponent("worker.yaml")
    }

    var logsDirURL: URL {
        fileManager.homeDirectoryForCurrentUser.appendingPathComponent("Library/Logs/nfrx", isDirectory: true)
    }

    var launchAgentURL: URL {
        fileManager.homeDirectoryForCurrentUser.appendingPathComponent("Library/LaunchAgents/io.nfrx.llm.plist")
    }

    func load() -> WorkerConfig {
        guard let data = try? String(contentsOf: configFileURL) else {
            return WorkerConfig()
        }
        return WorkerConfig.fromYAML(data)
    }

    func save(_ config: WorkerConfig) throws {
        try fileManager.createDirectory(at: configDirURL, withIntermediateDirectories: true)
        let yaml = config.toYAML()
        try yaml.write(to: configFileURL, atomically: true, encoding: .utf8)
    }

    func openConfigFolder() {
        NSWorkspace.shared.open(configDirURL)
    }

    func openLogsFolder() {
        NSWorkspace.shared.open(logsDirURL)
    }

    func copyDiagnostics() throws -> URL {
        let files = [
            logsDirURL.appendingPathComponent("worker.out"),
            logsDirURL.appendingPathComponent("worker.err"),
            configFileURL,
            launchAgentURL,
        ].filter { fileManager.fileExists(atPath: $0.path) }
        guard !files.isEmpty else {
            throw NSError(domain: "ConfigManager", code: 1, userInfo: [NSLocalizedDescriptionKey: "No diagnostic files found"])
        }
        let desktop = fileManager.homeDirectoryForCurrentUser.appendingPathComponent("Desktop", isDirectory: true)
        let zipURL = desktop.appendingPathComponent("NfrxDiagnostics.zip")
        try? fileManager.removeItem(at: zipURL)
        let process = Process()
        process.launchPath = "/usr/bin/zip"
        process.arguments = ["-j", zipURL.path] + files.map { $0.path }
        try process.run()
        process.waitUntilExit()
        if process.terminationStatus != 0 {
            throw NSError(domain: "ConfigManager", code: 2, userInfo: [NSLocalizedDescriptionKey: "Failed to create diagnostics zip"])
        }
        return zipURL
    }

    func loadToken() -> String? {
        let tokenURL = configDirURL.appendingPathComponent("worker.token")
        return try? String(contentsOf: tokenURL).trimmingCharacters(in: .whitespacesAndNewlines)
    }
}
