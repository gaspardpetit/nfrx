import Foundation

class ConfigManager {
    static let shared = ConfigManager()
    private let fileManager = FileManager.default
    private init() {}

    private var configDirURL: URL {
        fileManager.homeDirectoryForCurrentUser.appendingPathComponent("Library/Application Support/Llamapool", isDirectory: true)
    }

    private var configFileURL: URL {
        configDirURL.appendingPathComponent("worker.yaml")
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
        let logsURL = fileManager.homeDirectoryForCurrentUser.appendingPathComponent("Library/Logs/Llamapool", isDirectory: true)
        NSWorkspace.shared.open(logsURL)
    }

    func loadToken() -> String? {
        let tokenURL = configDirURL.appendingPathComponent("worker.token")
        return try? String(contentsOf: tokenURL).trimmingCharacters(in: .whitespacesAndNewlines)
    }
}
