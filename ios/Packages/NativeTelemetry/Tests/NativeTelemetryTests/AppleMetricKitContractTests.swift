import Foundation
import Testing
@testable import NativeTelemetry

struct AppleMetricKitContractTests {
    @Test(arguments: [NativeOperation.appCrash, .appHang])
    func minimalDiagnosticRetriesUntilCompatibleRelayIsAvailable(operation: NativeOperation) throws {
        let release = NativeRelease(release: "1.1", build: "165")!
        let envelope = NativeEnvelope(release: release, events: [NativeEvent(
            operation: operation, outcome: .failure, errorClass: operation == .appCrash ? .crash : .hang
        )])
        let badRequest = try #require(HTTPURLResponse(
            url: URL(string: "https://example.test")!, statusCode: 400, httpVersion: nil, headerFields: nil
        ))
        let accepted = try #require(HTTPURLResponse(
            url: URL(string: "https://example.test")!, statusCode: 204, httpVersion: nil, headerFields: nil
        ))
        if case .retry(let delay) = NativeHTTPTransport.delivery(for: badRequest, envelope: envelope) {
            #expect(delay == 60)
        } else {
            Issue.record("older relay response would discard a valid minimal diagnostic")
        }
        if case .accepted = NativeHTTPTransport.delivery(for: accepted, envelope: envelope) {
        } else {
            Issue.record("compatible relay acknowledgment was not accepted")
        }
        let ordinary = NativeEnvelope(release: release, events: [NativeEvent(operation: .apiRequest)])
        if case .rejected = NativeHTTPTransport.delivery(for: badRequest, envelope: ordinary) {
        } else {
            Issue.record("ordinary invalid event was retried")
        }
        let invalid = NativeEnvelope(release: release, events: [NativeEvent(operation: .appCrash)])
        if case .rejected = NativeHTTPTransport.delivery(for: badRequest, envelope: invalid) {
        } else {
            Issue.record("invalid diagnostic outcome was retried")
        }
    }

    @Test(arguments: [false, true])
    func systemOnlyDiagnosticPersistsMinimalFailure(hang: Bool) async throws {
        let fixture = try #require(Bundle.module.url(
            forResource: "AppleCallStackTree", withExtension: "json", subdirectory: "Fixtures"
        ))
        let data = try Data(contentsOf: fixture)
        let object = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
        let tree = try #require(object["callStackTree"] as? [String: Any])
        let stacks = try #require(tree["callStacks"] as? [[String: Any]])
        let roots = try #require(stacks.first?["callStackRootFrames"] as? [[String: Any]])
        let systemFrames = try #require(roots.first?["subFrames"] as? [[String: Any]])
        let systemOnly = try JSONSerialization.data(withJSONObject: [
            "callStackTree": ["callStacks": [["threadAttributed": true, "callStackRootFrames": systemFrames]]]
        ])
        let timestamp = Date()
        let event = MetricKitProjection.event(
            stackJSON: systemOnly, executableName: "MetricKitTestApp", platformArchitecture: "arm64",
            timestamp: timestamp, hang: hang
        )
        #expect(event.operation == (hang ? .appHang : .appCrash))
        #expect(event.outcome == .failure)
        #expect(event.errorClass == (hang ? .hang : .crash))
        #expect(event.crash == nil)
        #expect(event.timestamp == timestamp)
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let queue = try NativeTelemetryQueue(directory: directory)
        try await queue.enqueue(event, release: NativeRelease(release: "1.0", build: "100")!)
        let restarted = try NativeTelemetryQueue(directory: directory)
        let transport = RecordingTransport()
        try await restarted.drain(using: transport)
        let envelope = try #require(await transport.deliveries.first)
        #expect(envelope.release == "1.0")
        #expect(envelope.build == "100")
        #expect(envelope.events.first?.id == event.id)
        #expect(envelope.events.first?.outcome == .failure)
        let serialized = try #require(String(data: NativeCoding.encoder().encode(envelope), encoding: .utf8))
        #expect(!serialized.contains("libdyld"))
        #expect(!serialized.contains("7170808612"))
        #expect(!serialized.contains("image_uuid"))
    }

    @Test func unsupportedArchitectureStillEmitsFailureWithoutInventedSymbols() throws {
        let fixture = try #require(Bundle.module.url(
            forResource: "AppleCallStackTree", withExtension: "json", subdirectory: "Fixtures"
        ))
        let event = MetricKitProjection.event(
            stackJSON: try Data(contentsOf: fixture), executableName: "MetricKitTestApp",
            platformArchitecture: "unsupported-fixture", timestamp: Date(), hang: false
        )
        #expect(event.operation == .appCrash)
        #expect(event.outcome == .failure)
        #expect(event.errorClass == .crash)
        #expect(event.crash == nil)
        let serialized = try #require(String(data: NativeCoding.encoder().encode(event), encoding: .utf8))
        #expect(!serialized.contains("unsupported-fixture"))
    }

    @Test func documentedCallStackTreeMethodOutputProjectsAppFrame() throws {
        let fixture = try #require(Bundle.module.url(
            forResource: "AppleCallStackTree", withExtension: "json", subdirectory: "Fixtures"
        ))
        let data = try Data(contentsOf: fixture)
        let architecture = try #require(NativeArchitecture(rawValue: "arm64"))
        let crash = try #require(MetricKitProjection.crash(
            stackJSON: data, executableName: "MetricKitTestApp", architecture: architecture
        ))
        #expect(crash.imageUUID.uuidString == "70B89F27-1634-3580-A695-57CDB41D7743")
        #expect(crash.frames.map(\.offset) == [165304])
        #expect(crash.architecture == .arm64)
        let projected = try #require(String(data: NativeCoding.encoder().encode(crash), encoding: .utf8))
        #expect(!projected.contains("7170766264"))
        #expect(!projected.contains("libdyld"))
    }
}
