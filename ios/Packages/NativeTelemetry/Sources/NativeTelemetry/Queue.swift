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
    enum StateCodingKeys: CodingKey { case pending, quarantine, losses }

    struct Entry: Codable, Sendable {
        let release: NativeRelease
        let event: NativeEvent
    }

    struct State: Codable {
        var pending: [Entry] = []
        var quarantine: [Entry] = []
        var losses: [Entry] = []

        init() {}

        init(from decoder: any Decoder) throws {
            let values = try decoder.container(keyedBy: StateCodingKeys.self)
            pending = try values.decode([Entry].self, forKey: .pending)
            quarantine = try values.decode([Entry].self, forKey: .quarantine)
            losses = try values.decodeIfPresent([Entry].self, forKey: .losses) ?? []
        }
    }

    private let file: URL
    private var state: State
    private var draining = false
    private var delivering: Set<UUID> = []
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
            guard state.pending.count + state.losses.count <= Self.capacity, state.quarantine.count <= 16,
                  state.losses.count <= 2 else {
                throw CocoaError(.fileReadCorruptFile)
            }
        } else {
            state = State()
        }
    }

    public func enqueue(_ event: NativeEvent, release: NativeRelease, now: Date = Date()) throws {
        guard event.timestamp <= now.addingTimeInterval(300),
              !state.pending.contains(where: { $0.event.id == event.id }),
              !state.quarantine.contains(where: { $0.event.id == event.id }) else { return }
        var next = state
        expire(&next, now: now)
        if event.timestamp < now.addingTimeInterval(-30 * 86_400) {
            reportLoss(.queueExpired, release: release, in: &next, now: now)
        } else {
            next.pending.append(Entry(release: release, event: event))
        }
        try trim(&next, now: now)
        try save(next)
    }

    public func counts() -> (pending: Int, quarantine: Int) {
        (state.pending.count + state.losses.count, state.quarantine.count)
    }

    public func drain(using transport: any NativeTransport, now: Date = Date()) async throws {
        guard !draining, now >= retryAt else { return }
        draining = true
        defer {
            draining = false
            delivering = []
        }
        while !Task.isCancelled {
            var next = state
            expire(&next, now: now)
            try trim(&next, now: now)
            if next.pending.count != state.pending.count
                || next.losses.map(\.event.id) != state.losses.map(\.event.id) {
                try save(next)
            }
            let candidates = state.losses + state.pending
            guard let first = candidates.first else { return }
            let batch = Array(candidates.prefix(while: { $0.release == first.release }).prefix(16))
            let envelope = NativeEnvelope(release: first.release, events: batch.map(\.event))
            delivering = Set(batch.map(\.event.id))
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
                next.losses.removeAll { ids.contains($0.event.id) }
                if case .rejected = delivery {
                    next.quarantine = Array((next.quarantine + batch).suffix(16))
                }
                try save(next)
            }
        }
    }

    private func trim(_ next: inout State, now: Date) throws {
        while try next.pending.count > Self.capacity - 2
            || NativeCoding.encoder().encode(next).count > Self.byteLimit {
            guard !next.pending.isEmpty else { break }
            let index = next.pending.firstIndex { $0.event.outcome != .failure && $0.event.crash == nil } ?? 0
            let dropped = next.pending.remove(at: index)
            reportLoss(.queueFull, release: dropped.release, in: &next, now: now)
        }
    }

    private func expire(_ next: inout State, now: Date) {
        let cutoff = now.addingTimeInterval(-30 * 86_400)
        if let expired = next.pending.first(where: { $0.event.timestamp < cutoff }) {
            next.pending.removeAll { $0.event.timestamp < cutoff }
            reportLoss(.queueExpired, release: expired.release, in: &next, now: now)
        }
        next.losses = next.losses.map { entry in
            guard entry.event.timestamp < cutoff else { return entry }
            return loss(entry.event.errorClass, release: entry.release, now: now)
        }
    }

    private func reportLoss(_ reason: NativeErrorClass, release: NativeRelease, in next: inout State, now: Date) {
        if let index = next.losses.firstIndex(where: { $0.event.errorClass == reason }) {
            if delivering.contains(next.losses[index].event.id) {
                next.losses[index] = loss(reason, release: release, now: now)
            }
            return
        }
        next.losses.append(loss(reason, release: release, now: now))
    }

    private func loss(_ reason: NativeErrorClass, release: NativeRelease, now: Date) -> Entry {
        Entry(release: release, event: NativeEvent(
            timestamp: now, operation: .telemetryDelivery, outcome: .failure, errorClass: reason
        ))
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
