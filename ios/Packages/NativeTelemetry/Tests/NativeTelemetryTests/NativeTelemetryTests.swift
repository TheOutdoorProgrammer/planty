import Foundation
import Testing
@testable import NativeTelemetry

actor RecordingTransport: NativeTransport {
    var deliveries: [NativeEnvelope] = []
    var response: NativeDelivery = .accepted
    var fails = false
    var delay: Duration = .zero

    func configure(response: NativeDelivery = .accepted, fails: Bool = false, delay: Duration = .zero) {
        self.response = response
        self.fails = fails
        self.delay = delay
    }

    func send(_ envelope: NativeEnvelope) async throws -> NativeDelivery {
        deliveries.append(envelope)
        if fails { throw URLError(.notConnectedToInternet) }
        try await Task.sleep(for: delay)
        return response
    }
}

struct NativeTelemetryTests {
    private let origin = NativeRelease(release: "1.1", build: "165")!

    @Test func replayPreservesSubsecondParentSpanTiming() throws {
        let timestamp = Date(timeIntervalSince1970: 1_789_000_000.375)
        let event = NativeEvent(timestamp: timestamp, operation: .apiRequest, durationMS: 125)
        let encoded = try NativeCoding.encoder().encode(event)
        let replay = try NativeCoding.decoder().decode(NativeEvent.self, from: encoded)
        #expect(abs(replay.timestamp.timeIntervalSince(timestamp)) < 0.001)
        #expect(replay.durationMS == 125)
    }

    @Test func metricKitProjectionDropsPrivateFieldsAndOtherBinaries() throws {
        let fixture = Data("""
        {"private":"notification-body-secret", "callStackTree":{"callStacks":[
          {"threadAttributed":false,"callStackRootFrames":[{
            "binaryName":"Planty","binaryUUID":"11111111-1111-1111-1111-111111111111",
            "offsetIntoBinaryTextSegment":1
          }]},
          {"threadAttributed":true,"callStackRootFrames":[
            {"binaryName":"system-private-path","binaryUUID":"22222222-2222-2222-2222-222222222222",
             "offsetIntoBinaryTextSegment":33,"subFrames":[
              {"binaryName":"Planty","binaryUUID":"33333333-3333-3333-3333-333333333333",
               "offsetIntoBinaryTextSegment":1234,"address":999999999,
               "exceptionMessage":"private-plant-name","subFrames":[
                {"binaryName":"Planty","binaryUUID":"33333333-3333-3333-3333-333333333333",
                 "offsetIntoBinaryTextSegment":5678}
              ]}
            ]}
          ]}
        ]}}
        """.utf8)
        let crash = try #require(MetricKitProjection.crash(
            stackJSON: fixture, executableName: "Planty", architecture: .arm64
        ))
        #expect(crash.frames.map(\.offset) == [1234, 5678])
        #expect(crash.imageUUID.uuidString == "33333333-3333-3333-3333-333333333333")
        let output = try #require(String(data: NativeCoding.encoder().encode(crash), encoding: .utf8))
        for privateValue in ["private", "notification", "999999999", "binaryName", "address"] {
            #expect(!output.contains(privateValue))
        }
        #expect(NativeRelease(release: "private-plant", build: "165") == nil)
        #expect(NativeRelease(release: "1.1", build: "token=value") == nil)
        #expect(NativeRelease(release: "1.1", build: "165.1") == nil)
        #expect(NativeRelease(release: "1.1", build: "1234567890123") == nil)
        #expect(NativeRelease(release: "1.1.1.1.1", build: "165") == nil)
        #expect(NativeRelease(release: "12345.1", build: "165") == nil)
    }

    @Test func offlineReplayRetainsOriginAndIDAcrossProcessRestart() async throws {
        let directory = temporaryDirectory()
        defer { try? FileManager.default.removeItem(at: directory) }
        let queue = try NativeTelemetryQueue(directory: directory)
        let event = NativeEvent(operation: .notificationOpen)
        try await queue.enqueue(event, release: origin)
        let offline = RecordingTransport()
        await offline.configure(fails: true)
        try await queue.drain(using: offline)
        #expect(await queue.counts().pending == 1)
        let restarted = try NativeTelemetryQueue(directory: directory)
        let online = RecordingTransport()
        try await restarted.drain(using: online)
        let delivery = try #require(await online.deliveries.first)
        #expect(delivery.release == "1.1")
        #expect(delivery.build == "165")
        #expect(delivery.events.first?.id == event.id)
        #expect(await restarted.counts().pending == 0)
        let serialized = try String(contentsOf: directory.appendingPathComponent("events.json"), encoding: .utf8)
        #expect(!serialized.contains(event.id.uuidString))
    }

    @Test func rejectionQuarantinesInsteadOfRetrying() async throws {
        let directory = temporaryDirectory()
        defer { try? FileManager.default.removeItem(at: directory) }
        let queue = try NativeTelemetryQueue(directory: directory)
        try await queue.enqueue(NativeEvent(operation: .apiRequest), release: origin)
        let transport = RecordingTransport()
        await transport.configure(response: .rejected)
        try await queue.drain(using: transport)
        try await queue.drain(using: transport)
        #expect(await transport.deliveries.count == 1)
        #expect(await queue.counts().pending == 0)
        #expect(await queue.counts().quarantine == 1)
    }

    @Test func concurrentDrainsDoNotRaceNewEvents() async throws {
        let directory = temporaryDirectory()
        defer { try? FileManager.default.removeItem(at: directory) }
        let queue = try NativeTelemetryQueue(directory: directory)
        try await queue.enqueue(NativeEvent(operation: .appStart), release: origin)
        let transport = RecordingTransport()
        await transport.configure(delay: .milliseconds(25))
        async let first: Void = queue.drain(using: transport)
        async let second: Void = queue.drain(using: transport)
        try await queue.enqueue(NativeEvent(operation: .notificationOpen), release: origin)
        _ = try await (first, second)
        let ids = await transport.deliveries.flatMap { $0.events.map(\.id) }
        #expect(ids.count == 2)
        #expect(Set(ids).count == 2)
        #expect(await queue.counts().pending == 0)
    }

    @Test func capacityAndExpiryAreBounded() async throws {
        let directory = temporaryDirectory()
        defer { try? FileManager.default.removeItem(at: directory) }
        let queue = try NativeTelemetryQueue(directory: directory)
        let now = Date()
        try await queue.enqueue(NativeEvent(
            timestamp: now.addingTimeInterval(-31 * 86_400), operation: .appStart
        ), release: origin)
        #expect(await queue.counts().pending == 1)
        for _ in 0...NativeTelemetryQueue.capacity {
            try await queue.enqueue(NativeEvent(operation: .apiRequest), release: origin)
        }
        #expect(await queue.counts().pending == NativeTelemetryQueue.capacity)
        let data = try Data(contentsOf: directory.appendingPathComponent("events.json"))
        #expect(data.count <= NativeTelemetryQueue.byteLimit)
        let transport = RecordingTransport()
        try await queue.drain(using: transport)
        #expect(await transport.deliveries.allSatisfy { $0.events.count <= 16 })
    }

    @Test func retryAfterPreventsImmediateStorm() async throws {
        let directory = temporaryDirectory()
        defer { try? FileManager.default.removeItem(at: directory) }
        let queue = try NativeTelemetryQueue(directory: directory)
        try await queue.enqueue(NativeEvent(operation: .appStart), release: origin)
        let transport = RecordingTransport()
        await transport.configure(response: .retry(after: 120))
        let now = Date()
        try await queue.drain(using: transport, now: now)
        try await queue.drain(using: transport, now: now.addingTimeInterval(60))
        #expect(await transport.deliveries.count == 1)
        await transport.configure()
        try await queue.drain(using: transport, now: now.addingTimeInterval(121))
        #expect(await queue.counts().pending == 0)
    }

    @Test func transportRejectsUnsafeDestinations() {
        #expect(NativeHTTPTransport(baseURL: URL(string: "http://example.test")!, bearer: "fake") == nil)
        var credentialURL = URLComponents(string: "https://example.test")!
        credentialURL.user = "fixture"
        #expect(NativeHTTPTransport(baseURL: credentialURL.url!, bearer: "fake") == nil)
        #expect(NativeHTTPTransport(baseURL: URL(string: "https://example.test?secret=value")!, bearer: "fake") == nil)
    }

    @Test func cancelledDrainRetainsUnacknowledgedEvent() async throws {
        let directory = temporaryDirectory()
        defer { try? FileManager.default.removeItem(at: directory) }
        let queue = try NativeTelemetryQueue(directory: directory)
        try await queue.enqueue(NativeEvent(operation: .appStart), release: origin)
        let transport = RecordingTransport()
        await transport.configure(delay: .seconds(30))
        let drain = Task { try await queue.drain(using: transport) }
        try await Task.sleep(for: .milliseconds(10))
        drain.cancel()
        try await drain.value
        #expect(await queue.counts().pending == 1)
        await transport.configure()
        try await queue.drain(using: transport, now: Date().addingTimeInterval(61))
        #expect(await queue.counts().pending == 0)
    }

    @Test func failedAtomicWritePreservesPreviouslyPersistedEvents() async throws {
        let directory = temporaryDirectory()
        let moved = temporaryDirectory()
        defer {
            try? FileManager.default.removeItem(at: directory)
            try? FileManager.default.removeItem(at: moved)
        }
        let queue = try NativeTelemetryQueue(directory: directory)
        let first = NativeEvent(operation: .notificationOpen)
        try await queue.enqueue(first, release: origin)
        try FileManager.default.moveItem(at: directory, to: moved)
        do {
            try await queue.enqueue(NativeEvent(operation: .apiRequest), release: origin)
            Issue.record("write unexpectedly succeeded without its directory")
        } catch {
            #expect(await queue.counts().pending == 1)
        }
        let replay = try NativeTelemetryQueue(directory: moved)
        let transport = RecordingTransport()
        try await replay.drain(using: transport)
        #expect(await transport.deliveries.first?.events.first?.id == first.id)
    }

    @Test func crashProjectionBoundsFramesAndRejectsMalformedOffsets() throws {
        let uuid = UUID().uuidString
        let frames: [[String: Any]] = (0..<100).map { offset in
            ["binaryName": "Planty", "binaryUUID": uuid, "offsetIntoBinaryTextSegment": offset]
        }
        let data = try JSONSerialization.data(withJSONObject: [
            "callStackTree": ["callStacks": [["callStackRootFrames": frames]]]
        ])
        let crash = try #require(MetricKitProjection.crash(
            stackJSON: data, executableName: "Planty", architecture: .arm64
        ))
        #expect(crash.frames.count == 64)
        for invalid in [-1, true, 1.5, UInt64(1) << 40] as [Any] {
            let malformed = try JSONSerialization.data(withJSONObject: [
                "callStackTree": ["callStacks": [["callStackRootFrames": [[
                    "binaryName": "Planty", "binaryUUID": uuid, "offsetIntoBinaryTextSegment": invalid
                ]]]]]
            ])
            #expect(MetricKitProjection.crash(
                stackJSON: malformed, executableName: "Planty", architecture: .arm64
            ) == nil)
        }
    }

    private func temporaryDirectory() -> URL {
        FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString, isDirectory: true)
    }
}
