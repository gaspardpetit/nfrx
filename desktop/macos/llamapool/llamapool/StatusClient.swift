import Foundation

struct WorkerStatus: Codable {
    enum State: String, Codable {
        case connectedIdle = "connected_idle"
        case connectedBusy = "connected_busy"
        case connecting
        case disconnected
        case draining
        case terminating
        case error
    }

    let state: State
    let connectedToServer: Bool
    let connectedToOllama: Bool
    let currentJobs: Int
    let maxConcurrency: Int
    let models: [String]
    let lastError: String
    let lastHeartbeat: String
    let workerId: String
    let workerName: String
    let version: String

    private enum CodingKeys: String, CodingKey {
        case state
        case connectedToServer = "connected_to_server"
        case connectedToOllama = "connected_to_ollama"
        case currentJobs = "current_jobs"
        case maxConcurrency = "max_concurrency"
        case models
        case lastError = "last_error"
        case lastHeartbeat = "last_heartbeat"
        case workerId = "worker_id"
        case workerName = "worker_name"
        case version
    }
}

class StatusClient {
    private let session: URLSession
    private var timer: Timer?
    private let url: URL
    var onUpdate: ((Result<WorkerStatus, Error>) -> Void)?

    init(port: Int = 4555, session: URLSession = .shared) {
        self.session = session
        self.url = URL(string: "http://127.0.0.1:\(port)/status")!
    }

    func start() {
        timer = Timer.scheduledTimer(withTimeInterval: 2, repeats: true) { [weak self] _ in
            self?.fetchStatus()
        }
        fetchStatus()
    }

    func stop() {
        timer?.invalidate()
        timer = nil
    }

    private func fetchStatus() {
        session.dataTask(with: url) { [weak self] data, _, error in
            guard let self = self else { return }
            if let error = error {
                self.onUpdate?(.failure(error))
                return
            }
            guard let data = data else {
                let err = NSError(domain: "StatusClient", code: -1, userInfo: [NSLocalizedDescriptionKey: "No data"])
                self.onUpdate?(.failure(err))
                return
            }
            do {
                let status = try JSONDecoder().decode(WorkerStatus.self, from: data)
                self.onUpdate?(.success(status))
            } catch {
                self.onUpdate?(.failure(error))
            }
        }.resume()
    }
}
