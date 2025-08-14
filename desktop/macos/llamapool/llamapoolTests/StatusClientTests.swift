import XCTest
@testable import llamapool

final class StatusClientTests: XCTestCase {
    let sampleJSON = """
    {
      "state": "connected_idle",
      "connected_to_server": true,
      "connected_to_ollama": true,
      "current_jobs": 1,
      "max_concurrency": 2,
      "models": ["llama3"],
      "last_error": "",
      "last_heartbeat": "2024-05-01T12:00:00Z",
      "worker_id": "1234",
      "worker_name": "test-worker",
      "version": "v0.0.1"
    }
    """

    func testDecodeSample() throws {
        let data = Data(sampleJSON.utf8)
        let status = try JSONDecoder().decode(WorkerStatus.self, from: data)
        XCTAssertEqual(status.state, .connectedIdle)
        XCTAssertEqual(status.workerName, "test-worker")
        XCTAssertEqual(status.currentJobs, 1)
    }

    func testPollingWithMockServer() {
        class URLProtocolMock: URLProtocol {
            static var data: Data?
            override class func canInit(with request: URLRequest) -> Bool { true }
            override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
            override func startLoading() {
                if let data = URLProtocolMock.data {
                    self.client?.urlProtocol(self, didLoad: data)
                    self.client?.urlProtocol(self, didReceive: HTTPURLResponse(url: request.url!, statusCode: 200, httpVersion: nil, headerFields: nil)!, cacheStoragePolicy: .notAllowed)
                    self.client?.urlProtocolDidFinishLoading(self)
                }
            }
            override func stopLoading() {}
        }

        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [URLProtocolMock.self]
        URLProtocolMock.data = Data(sampleJSON.utf8)
        let session = URLSession(configuration: config)
        let client = StatusClient(session: session)
        let expect = expectation(description: "Status")
        client.onUpdate = { result in
            if case .success(let status) = result {
                XCTAssertEqual(status.workerId, "1234")
                expect.fulfill()
            }
        }
        client.start()
        waitForExpectations(timeout: 1)
        client.stop()
    }
}
