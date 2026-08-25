import Foundation

struct Actuator: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let entityID: String
    var name: String
    let kind: ActuatorKind
    var plantIDs: [UUID]
    let createdAt: Date
    let updatedAt: Date
    var activeLease: ActuatorLease?

    enum CodingKeys: String, CodingKey {
        case id
        case entityID = "entity_id"
        case name
        case kind
        case plantIDs = "plant_ids"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
        case activeLease = "active_lease"
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

    enum CodingKeys: String, CodingKey {
        case name
        case plantIDs = "plant_ids"
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
