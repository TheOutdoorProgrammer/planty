import Foundation

struct Actuator: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let entityID: String
    var name: String
    let kind: ActuatorKind
    var plantIDs: [UUID]
    var policyControlEnabled: Bool = false
    let createdAt: Date
    let updatedAt: Date
    var activeLease: ActuatorLease?
    var lightSchedule: LightSchedule?

    enum CodingKeys: String, CodingKey {
        case id
        case entityID = "entity_id"
        case name
        case kind
        case plantIDs = "plant_ids"
        case policyControlEnabled = "policy_control_enabled"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
        case activeLease = "active_lease"
        case lightSchedule = "light_schedule"
    }
}

extension Actuator {
    init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(UUID.self, forKey: .id)
        entityID = try container.decode(String.self, forKey: .entityID)
        name = try container.decode(String.self, forKey: .name)
        kind = try container.decode(ActuatorKind.self, forKey: .kind)
        plantIDs = try container.decode([UUID].self, forKey: .plantIDs)
        policyControlEnabled = try container.decodeIfPresent(Bool.self, forKey: .policyControlEnabled) ?? false
        createdAt = try container.decode(Date.self, forKey: .createdAt)
        updatedAt = try container.decode(Date.self, forKey: .updatedAt)
        activeLease = try container.decodeIfPresent(ActuatorLease.self, forKey: .activeLease)
        lightSchedule = try container.decodeIfPresent(LightSchedule.self, forKey: .lightSchedule)
    }
}

struct LightSchedule: Codable, Sendable, Hashable {
    let actuatorID: UUID
    let startMinute: Int
    let endMinute: Int
    let timezone: String
    let enabled: Bool
    let lastAppliedState: Bool?
    let lastAppliedAt: Date?
    let lastError: String?
    let createdAt: Date
    let updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case actuatorID = "actuator_id"
        case startMinute = "start_minute"
        case endMinute = "end_minute"
        case timezone
        case enabled
        case lastAppliedState = "last_applied_state"
        case lastAppliedAt = "last_applied_at"
        case lastError = "last_error"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

struct LightScheduleRequest: Codable, Sendable, Equatable {
    let startMinute: Int
    let endMinute: Int
    let timezone: String
    let enabled: Bool
    let actor: String

    enum CodingKeys: String, CodingKey {
        case startMinute = "start_minute"
        case endMinute = "end_minute"
        case timezone, enabled, actor
    }
}

struct LightStateRequest: Codable, Sendable, Equatable {
    let isOn: Bool
    let actor: String

    enum CodingKeys: String, CodingKey {
        case isOn = "on"
        case actor
    }
}

struct LightStateResponse: Decodable, Sendable {
    let isOn: Bool

    enum CodingKeys: String, CodingKey {
        case isOn = "on"
    }
}

extension Sequence where Element == Actuator {
    func assigned(to plantID: UUID) -> [Actuator] {
        filter { $0.plantIDs.contains(plantID) }
    }
}

enum ActuatorRunDuration: Int, CaseIterable, Identifiable {
    case fiveMinutes = 300
    case tenMinutes = 600
    case fifteenMinutes = 900
    case thirtyMinutes = 1_800
    case oneHour = 3_600

    var id: Int { rawValue }

    var label: String {
        switch self {
        case .fiveMinutes: "5 minutes"
        case .tenMinutes: "10 minutes"
        case .fifteenMinutes: "15 minutes"
        case .thirtyMinutes: "30 minutes"
        case .oneHour: "1 hour"
        }
    }
}

struct ActuatorLease: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let actuatorID: UUID
    let requestedSeconds: Int
    let deadline: Date
    let actor: String
    let source: ObservationSource
    let idempotencyKey: UUID
    let startedAt: Date?
    let stoppedAt: Date?
    let stopReason: String?
    let createdAt: Date

    var isActive: Bool { stoppedAt == nil }

    enum CodingKeys: String, CodingKey {
        case id
        case actuatorID = "actuator_id"
        case requestedSeconds = "requested_seconds"
        case deadline
        case actor
        case source
        case idempotencyKey = "idempotency_key"
        case startedAt = "started_at"
        case stoppedAt = "stopped_at"
        case stopReason = "stop_reason"
        case createdAt = "created_at"
    }
}

struct ActuatorEvent: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let actuatorID: UUID
    let leaseID: UUID?
    let action: ActuatorEventAction
    let actor: String
    let source: ObservationSource
    let idempotencyKey: UUID?
    let detail: String?
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case actuatorID = "actuator_id"
        case leaseID = "lease_id"
        case action
        case actor
        case source
        case idempotencyKey = "idempotency_key"
        case detail
        case createdAt = "created_at"
    }
}

struct ActuatorListResponse: Codable, Sendable {
    let actuators: [Actuator]
}

struct ActuatorEventListResponse: Codable, Sendable {
    let events: [ActuatorEvent]
}

struct DiscoveredActuatorListResponse: Codable, Sendable {
    let entities: [HomeAssistantEntity]
}

struct ActuatorRegistration: Codable, Sendable, Equatable {
    let entityID: String
    let name: String
    let plantIDs: [UUID]

    enum CodingKeys: String, CodingKey {
        case entityID = "entity_id"
        case name
        case plantIDs = "plant_ids"
    }
}

struct ActuatorRename: Codable, Sendable {
    let name: String
    let plantIDs: [UUID]
    let policyControlEnabled: Bool

    enum CodingKeys: String, CodingKey {
        case name
        case plantIDs = "plant_ids"
        case policyControlEnabled = "policy_control_enabled"
    }
}

struct ActuatorStartRequest: Codable, Sendable, Equatable {
    let durationSeconds: Int
    let actor: String
    let idempotencyKey: UUID

    enum CodingKeys: String, CodingKey {
        case durationSeconds = "duration_seconds"
        case actor
        case idempotencyKey = "idempotency_key"
    }
}

struct ActuatorStopRequest: Codable, Sendable, Equatable {
    let actor: String
    let idempotencyKey: UUID

    enum CodingKeys: String, CodingKey {
        case actor
        case idempotencyKey = "idempotency_key"
    }
}

struct ActuatorStopResponse: Codable, Sendable {
    let stopped: Bool
}
