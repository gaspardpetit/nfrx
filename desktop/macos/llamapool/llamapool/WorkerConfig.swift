import Foundation

struct WorkerConfig: Equatable {
    var serverURL: String
    var workerKey: String
    var ollamaBaseURL: String
    var maxConcurrency: Int
    var statusPort: Int

    init(serverURL: String = "", workerKey: String = "", ollamaBaseURL: String = "", maxConcurrency: Int = 1, statusPort: Int = 4555) {
        self.serverURL = serverURL
        self.workerKey = workerKey
        self.ollamaBaseURL = ollamaBaseURL
        self.maxConcurrency = maxConcurrency
        self.statusPort = statusPort
    }

    func toYAML() -> String {
        return """
server_url: \(serverURL)
worker_key: \(workerKey)
ollama_base_url: \(ollamaBaseURL)
max_concurrency: \(maxConcurrency)
status_port: \(statusPort)
"""
    }

    static func fromYAML(_ yaml: String) -> WorkerConfig {
        var dict: [String: String] = [:]
        yaml.split(separator: "\n").forEach { line in
            let parts = line.split(separator: ":", maxSplits: 1).map { String($0).trimmingCharacters(in: .whitespaces) }
            if parts.count == 2 {
                dict[parts[0]] = parts[1]
            }
        }
        let serverURL = dict["server_url"] ?? ""
        let workerKey = dict["worker_key"] ?? ""
        let ollamaBaseURL = dict["ollama_base_url"] ?? ""
        let maxConcurrency = Int(dict["max_concurrency"] ?? "1") ?? 1
        let statusPort = Int(dict["status_port"] ?? "4555") ?? 4555
        return WorkerConfig(serverURL: serverURL, workerKey: workerKey, ollamaBaseURL: ollamaBaseURL, maxConcurrency: maxConcurrency, statusPort: statusPort)
    }

    func isValid() -> Bool {
        return !serverURL.isEmpty && maxConcurrency > 0 && statusPort > 0
    }
}
