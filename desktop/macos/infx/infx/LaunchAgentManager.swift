import Foundation

class LaunchAgentManager {
    static let shared = LaunchAgentManager()

    private let fileManager = FileManager.default
    private let label = "io.infx.llm"

    private var launchAgentURL: URL {
        fileManager.homeDirectoryForCurrentUser.appendingPathComponent("Library/LaunchAgents/\(label).plist")
    }

    private var logsDirURL: URL {
        fileManager.homeDirectoryForCurrentUser.appendingPathComponent("Library/Logs/infx")
    }

    private var configDirURL: URL {
        fileManager.homeDirectoryForCurrentUser.appendingPathComponent("Library/Application Support/infx")
    }

    private var workerBinaryURL: URL {
        Bundle.main.resourceURL!.appendingPathComponent("bin/infx-llm")
    }

    private var templateURL: URL? {
        Bundle.main.url(forResource: "io.infx.llm", withExtension: "plist.template")
    }

    func start() throws {
        try installAgentIfNeeded()
        try runLaunchctl(["load", launchAgentURL.path])
        try runLaunchctl(["start", label])
    }

    func stop() throws {
        try runLaunchctl(["stop", label])
        try runLaunchctl(["unload", launchAgentURL.path])
    }

    func setRunAtLoad(_ enabled: Bool) throws {
        try installAgentIfNeeded()
        if let dict = NSMutableDictionary(contentsOf: launchAgentURL) {
            dict["RunAtLoad"] = enabled
            dict.write(to: launchAgentURL, atomically: true)
            try runLaunchctl(["unload", launchAgentURL.path])
            try runLaunchctl(["load", launchAgentURL.path])
        }
    }

    func isRunAtLoadEnabled() -> Bool {
        if let dict = NSDictionary(contentsOf: launchAgentURL) {
            return dict["RunAtLoad"] as? Bool ?? false
        }
        return false
    }

    func configDirectory() -> URL {
        configDirURL
    }

    func logsDirectory() -> URL {
        logsDirURL
    }

    func isAgentLoaded() -> Bool {
        (try? runLaunchctl(["list", label]).exitCode) == 0
    }

    private func installAgentIfNeeded() throws {
        if !fileManager.fileExists(atPath: launchAgentURL.path) {
            try fileManager.createDirectory(at: launchAgentURL.deletingLastPathComponent(), withIntermediateDirectories: true)
            try fileManager.createDirectory(at: logsDirURL, withIntermediateDirectories: true)
            try fileManager.createDirectory(at: configDirURL, withIntermediateDirectories: true)
            if let templateURL = templateURL {
                var content = try String(contentsOf: templateURL)
                content = content.replacingOccurrences(of: "{{WORKER_PATH}}", with: workerBinaryURL.path)
                content = content.replacingOccurrences(of: "{{LOG_DIR}}", with: logsDirURL.path)
                try content.write(to: launchAgentURL, atomically: true, encoding: .utf8)
            }
        }
    }

    @discardableResult
    private func runLaunchctl(_ arguments: [String]) throws -> (output: String, exitCode: Int32) {
        let process = Process()
        process.launchPath = "/bin/launchctl"
        process.arguments = arguments
        let pipe = Pipe()
        process.standardOutput = pipe
        process.standardError = pipe
        try process.run()
        process.waitUntilExit()
        let data = pipe.fileHandleForReading.readDataToEndOfFile()
        let output = String(data: data, encoding: .utf8) ?? ""
        return (output, process.terminationStatus)
    }
}
