import Foundation

public enum NativeOperation: String, Codable, Sendable {
    case appStart = "app.start"
    case notificationOpen = "notification.open"
    case apiRequest = "api.request"
    case appCrash = "app.crash"
    case appHang = "app.hang"
    case telemetryDelivery = "telemetry.delivery"
}

public enum NativeOutcome: String, Codable, Sendable {
    case success, failure, cancelled
}

public enum NativeErrorClass: String, Codable, Sendable {
    case none = ""
    case transport, timeout, unauthorized, server, decoding, crash, hang, other
    case queueFull = "queue_full"
    case queueExpired = "queue_expired"
}

public enum NativeArchitecture: String, Codable, Sendable {
    case arm64, arm64e
    case x86 = "x86_64"
}

public struct NativeRelease: Codable, Sendable, Equatable {
    public let release: String
    public let build: String

    public init?(release: String, build: String) {
        func digits(_ value: some StringProtocol, maximum: Int) -> Bool {
            !value.isEmpty && value.utf8.count <= maximum && value.utf8.allSatisfy { (48...57).contains($0) }
        }
        let components = release.split(separator: ".", omittingEmptySubsequences: false)
        guard (1...4).contains(components.count), components.allSatisfy({ digits($0, maximum: 4) }),
              digits(build, maximum: 12) else { return nil }
        self.release = release
        self.build = build
    }
}

public struct NativeCrash: Codable, Sendable, Equatable {
    public struct Frame: Codable, Sendable, Equatable {
        public let offset: UInt64
        public init(offset: UInt64) { self.offset = offset }
    }

    public let imageUUID: UUID
    public let architecture: NativeArchitecture
    public let frames: [Frame]

    public init?(imageUUID: UUID, architecture: NativeArchitecture, frames: [Frame]) {
        guard !frames.isEmpty, frames.count <= 64,
              imageUUID.uuidString != "00000000-0000-0000-0000-000000000000",
              frames.allSatisfy({ $0.offset < 1 << 40 }) else { return nil }
        self.imageUUID = imageUUID
        self.architecture = architecture
        self.frames = frames
    }

    enum CodingKeys: String, CodingKey {
        case imageUUID = "image_uuid"
        case architecture, frames
    }
}

public struct NativeEvent: Codable, Sendable, Equatable {
    public let id: UUID
    public let timestamp: Date
    public let operation: NativeOperation
    public let outcome: NativeOutcome
    public let durationMS: Int
    public let errorClass: NativeErrorClass
    public let crash: NativeCrash?

    public init(
        id: UUID = UUID(), timestamp: Date = Date(), operation: NativeOperation,
        outcome: NativeOutcome = .success, durationMS: Int = 0,
        errorClass: NativeErrorClass = .none, crash: NativeCrash? = nil
    ) {
        self.id = id
        self.timestamp = timestamp
        self.operation = operation
        self.outcome = outcome
        self.durationMS = min(max(durationMS, 0), 3_600_000)
        self.errorClass = errorClass
        self.crash = crash
    }

    enum CodingKeys: String, CodingKey {
        case id, timestamp, operation, outcome, crash
        case durationMS = "duration_ms"
        case errorClass = "error_class"
    }
}

public struct NativeEnvelope: Codable, Sendable {
    public let schemaVersion = 1
    public let release: String
    public let build: String
    public let events: [NativeEvent]

    public init(release: NativeRelease, events: [NativeEvent]) {
        self.release = release.release
        self.build = release.build
        self.events = events
    }

    enum CodingKeys: String, CodingKey {
        case schemaVersion = "schema_version"
        case release, build, events
    }
}

public enum NativeCoding {
    public static func encoder() -> JSONEncoder {
        let encoder = JSONEncoder()
        let formatter = Date.ISO8601FormatStyle(includingFractionalSeconds: true)
        encoder.dateEncodingStrategy = .custom { date, encoder in
            var value = encoder.singleValueContainer()
            try value.encode(formatter.format(date))
        }
        encoder.outputFormatting = [.sortedKeys]
        return encoder
    }

    public static func decoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        let fractional = Date.ISO8601FormatStyle(includingFractionalSeconds: true)
        let legacy = Date.ISO8601FormatStyle()
        decoder.dateDecodingStrategy = .custom { decoder in
            let value = try decoder.singleValueContainer()
            let text = try value.decode(String.self)
            guard let date = (try? fractional.parse(text)) ?? (try? legacy.parse(text)) else {
                throw DecodingError.dataCorruptedError(in: value, debugDescription: "Invalid telemetry timestamp")
            }
            return date
        }
        return decoder
    }
}
