import XCTest
@testable import llamapool

final class ConfigTests: XCTestCase {
    func testYAMLRoundTrip() throws {
        let config = WorkerConfig(serverURL: "wss://example", clientKey: "key", completionBaseURL: "http://localhost/v1", maxConcurrency: 2, statusPort: 4555)
        let yaml = config.toYAML()
        let decoded = WorkerConfig.fromYAML(yaml)
        XCTAssertEqual(config, decoded)
    }

    func testValidationFailsOnEmptyServer() {
        let config = WorkerConfig()
        XCTAssertFalse(config.isValid())
    }
}
