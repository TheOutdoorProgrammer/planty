import Foundation
import Testing
@testable import NativeTelemetry

private actor PausedFirstDelivery: NativeTransport {
    var envelopes: [NativeEnvelope] = []
    private var observer: CheckedContinuation<Void, Never>?
    private var release: CheckedContinuation<Void, Never>?

    func send(_ envelope: NativeEnvelope) async throws -> NativeDelivery {
        envelopes.append(envelope)
        if envelopes.count == 1 {
            observer?.resume()
            await withCheckedContinuation { release = $0 }
        }
        return .accepted
    }

    func waitForFirstBatch() async {
        if envelopes.isEmpty { await withCheckedContinuation { observer = $0 } }
    }

    func resume() { release?.resume() }
}

struct QueuePressureTests {
    private let origin = NativeRelease(release: "1.1", build: "165")!

    @Test func routineFloodPreservesCrashAndReportsCapacityLossAfterRestart() async throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let queue = try NativeTelemetryQueue(directory: directory)
        let crash = NativeEvent(operation: .appCrash, outcome: .failure, errorClass: .crash)
        try await queue.enqueue(crash, release: origin)
        for _ in 0..<(NativeTelemetryQueue.capacity + 20) {
            try await queue.enqueue(NativeEvent(operation: .apiRequest), release: origin)
        }
        #expect(await queue.counts().pending <= NativeTelemetryQueue.capacity)
        let restarted = try NativeTelemetryQueue(directory: directory)
        let transport = RecordingTransport()
        try await restarted.drain(using: transport)
        let events = await transport.deliveries.flatMap(\.events)
        #expect(events.contains { $0.id == crash.id })
        let loss = try #require(events.first { $0.operation == .telemetryDelivery })
        #expect(loss.errorClass == .queueFull)
        #expect(loss.outcome == .failure)
        #expect(events.filter { $0.operation == .telemetryDelivery }.count == 1)
    }

    @Test func fullFailureQueueRemainsBoundedAndPersistsLossReport() async throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let queue = try NativeTelemetryQueue(directory: directory)
        for _ in 0..<(NativeTelemetryQueue.capacity + 20) {
            try await queue.enqueue(NativeEvent(
                operation: .apiRequest, outcome: .failure, errorClass: .timeout
            ), release: origin)
        }
        let offline = RecordingTransport()
        await offline.configure(fails: true)
        try await queue.drain(using: offline)
        #expect(await queue.counts().pending <= NativeTelemetryQueue.capacity)
        let data = try Data(contentsOf: directory.appendingPathComponent("events.json"))
        #expect(data.count <= NativeTelemetryQueue.byteLimit)
        let restarted = try NativeTelemetryQueue(directory: directory)
        let online = RecordingTransport()
        try await restarted.drain(using: online)
        #expect(await online.deliveries.flatMap(\.events).contains { $0.errorClass == .queueFull })
        #expect(await restarted.counts().pending == 0)
    }

    @Test func offlineExpiryReportsLossWithFreshTimestampOnRecovery() async throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let now = Date()
        let queue = try NativeTelemetryQueue(directory: directory)
        try await queue.enqueue(NativeEvent(timestamp: now, operation: .notificationOpen), release: origin, now: now)
        let offline = RecordingTransport()
        await offline.configure(fails: true)
        try await queue.drain(using: offline, now: now.addingTimeInterval(31 * 86_400))
        let restarted = try NativeTelemetryQueue(directory: directory)
        let online = RecordingTransport()
        let recovered = now.addingTimeInterval(62 * 86_400)
        try await restarted.drain(using: online, now: recovered)
        let events = await online.deliveries.flatMap(\.events)
        #expect(events.count == 1)
        #expect(events.first?.errorClass == .queueExpired)
        #expect(events.first?.timestamp == recovered)
    }

    @Test func acknowledgmentCannotClearLossesThatOccurredDuringDelivery() async throws {
        let directory = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString)
        defer { try? FileManager.default.removeItem(at: directory) }
        let queue = try NativeTelemetryQueue(directory: directory)
        for _ in 0..<NativeTelemetryQueue.capacity {
            try await queue.enqueue(NativeEvent(operation: .apiRequest), release: origin)
        }
        let transport = PausedFirstDelivery()
        let drain = Task { try await queue.drain(using: transport) }
        await transport.waitForFirstBatch()
        try await queue.enqueue(NativeEvent(operation: .apiRequest), release: origin)
        await transport.resume()
        try await drain.value
        let losses = await transport.envelopes.flatMap(\.events).filter { $0.errorClass == .queueFull }
        #expect(losses.count == 2)
        #expect(Set(losses.map(\.id)).count == 2)
        #expect(await queue.counts().pending == 0)
    }
}
