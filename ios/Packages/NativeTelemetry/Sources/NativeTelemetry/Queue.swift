import Foundation

public enum NativeDelivery: Sendable {
    case accepted
    case retry(after: TimeInterval)
    case rejected
}

public protocol NativeTransport: Sendable {
    func send(_ envelope: NativeEnvelope) async throws -> NativeDelivery
}

public actor NativeTelemetryQueue {
    struct Entry: Codable, Sendable {
        let release: NativeRelease
        let event: NativeEvent
    }

    struct State: Codable {
        var pending: [Entry] = []
        var quarantine: [Entry] = []
    }

    private let file: URL
    private var state: State
    private var draining = false
    private var retryAt = Date.distantPast
    public static let capacity = 256
    public static let byteLimit = 524_288

    public init(directory: URL) throws {
        try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
        var directory = directory
        var attributes = URLResourceValues()
        attributes.isExcludedFromBackup = true
        try directory.setResourceValues(attributes)
        file = directory.appendingPathComponent("events.json")
        if FileManager.default.fileExists(atPath: file.path) {
            let size = try file.resourceValues(forKeys: [.fileSizeKey]).fileSize ?? 0
            guard size <= Self.byteLimit else { throw CocoaError(.fileReadTooLarge) }
            state = try NativeCoding.decoder().decode(State.self, from: Data(contentsOf: file))
            guard state.pending.count <= Self.capacity, state.quarantine.count <= 16 else {
                throw CocoaError(.fileReadCorruptFile)
            }
        } else {
            state = State()
        }
    }

    public func enqueue(_ event: NativeEvent, release: NativeRelease, now: Date = Date()) throws {
        guard event.timestamp >= now.addingTimeInterval(-30 * 86_400),
              event.timestamp <= now.addingTimeInterval(300),
              !state.pending.contains(where: { $0.event.id == event.id }),
              !state.quarantine.contains(where: { $0.event.id == event.id }) else { return }
        var next = state
        next.pending.removeAll { $0.event.timestamp < now.addingTimeInterval(-30 * 86_400) }
        next.pending.append(Entry(release: release, event: event))
        if next.pending.count > Self.capacity {
            next.pending.removeFirst(next.pending.count - Self.capacity)
        }
        while try NativeCoding.encoder().encode(next).count > Self.byteLimit, next.pending.count > 1 {
            next.pending.removeFirst()
        }
        try save(next)
    }

    public func counts() -> (pending: Int, quarantine: Int) {
        (state.pending.count, state.quarantine.count)
    }

    public func drain(using transport: any NativeTransport, now: Date = Date()) async throws {
        guard !draining, now >= retryAt else { return }
        draining = true
        defer { draining = false }
        while !Task.isCancelled, let first = state.pending.first {
            var next = state
            next.pending.removeAll { $0.event.timestamp < now.addingTimeInterval(-30 * 86_400) }
            if next.pending.count != state.pending.count {
                try save(next)
                continue
            }
            let batch = Array(state.pending.prefix(while: { $0.release == first.release }).prefix(16))
            let envelope = NativeEnvelope(release: first.release, events: batch.map(\.event))
            let delivery: NativeDelivery
            do {
                delivery = try await transport.send(envelope)
            } catch {
                retryAt = now.addingTimeInterval(60)
                return
            }
            switch delivery {
            case .retry(let delay):
                retryAt = now.addingTimeInterval(min(max(delay, 60), 3_600))
                return
            case .accepted, .rejected:
                let ids = Set(batch.map(\.event.id))
                next = state
                next.pending.removeAll { ids.contains($0.event.id) }
                if case .rejected = delivery {
                    next.quarantine = Array((next.quarantine + batch).suffix(16))
                }
                try save(next)
            }
        }
    }

    private func save(_ next: State) throws {
        let data = try NativeCoding.encoder().encode(next)
        guard data.count <= Self.byteLimit else { throw CocoaError(.fileWriteOutOfSpace) }
        #if os(iOS)
        try data.write(to: file, options: [.atomic, .completeFileProtectionUntilFirstUserAuthentication])
        #else
        try data.write(to: file, options: .atomic)
        #endif
        state = next
    }
}
