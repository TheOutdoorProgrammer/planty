import Foundation

struct IncidentEvidence: Codable, Sendable, Hashable {
    let runID: UUID
    let verdictIDs: [UUID]
    let observationIDs: [UUID]
    let sensorLinkIDs: [UUID]
    let actuatorEventIDs: [UUID]
    let note: String

    enum CodingKeys: String, CodingKey {
        case runID = "run_id"
        case verdictIDs = "verdict_ids"
        case observationIDs = "observation_ids"
        case sensorLinkIDs = "sensor_link_ids"
        case actuatorEventIDs = "actuator_event_ids"
        case note
    }

    init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        runID = try container.decode(UUID.self, forKey: .runID)
        verdictIDs = try container.decodeIfPresent([UUID].self, forKey: .verdictIDs) ?? []
        observationIDs = try container.decodeIfPresent([UUID].self, forKey: .observationIDs) ?? []
        sensorLinkIDs = try container.decodeIfPresent([UUID].self, forKey: .sensorLinkIDs) ?? []
        actuatorEventIDs = try container.decodeIfPresent([UUID].self, forKey: .actuatorEventIDs) ?? []
        note = try container.decodeIfPresent(String.self, forKey: .note) ?? ""
    }
}

struct IncidentPlant: Codable, Sendable, Hashable, Identifiable {
    let plant: Plant
    let role: String
    let verdictID: UUID
    let action: VerdictAction
    let confidence: Double
    let firstSeenAt: Date
    let lastSeenAt: Date

    var id: UUID { plant.id }

    enum CodingKeys: String, CodingKey {
        case plant, role, action, confidence
        case verdictID = "verdict_id"
        case firstSeenAt = "first_seen_at"
        case lastSeenAt = "last_seen_at"
    }
}

struct GardenIncident: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let status: IncidentStatus
    let suspectedFactorType: IncidentFactor
    let suspectedFactorRef: String
    let summary: String
    private let providedReason: String?
    let confidence: Double
    let evidence: IncidentEvidence
    let detectedRunID: UUID
    let plants: [IncidentPlant]
    let firstSeenAt: Date
    let lastSeenAt: Date
    let acknowledgedAt: Date?
    let acknowledgedBy: String?
    let resolvedAt: Date?
    let resolvedBy: String?
    let resolution: IncidentResolution?
    let conclusion: String?
    let createdAt: Date
    let updatedAt: Date

    var reason: String { providedReason?.nilIfBlank ?? summary }

    enum CodingKeys: String, CodingKey {
        case id, status, summary, confidence, evidence, plants, resolution, conclusion
        case providedReason = "reason"
        case suspectedFactorType = "suspected_factor_type"
        case suspectedFactorRef = "suspected_factor_ref"
        case detectedRunID = "detected_run_id"
        case firstSeenAt = "first_seen_at"
        case lastSeenAt = "last_seen_at"
        case acknowledgedAt = "acknowledged_at"
        case acknowledgedBy = "acknowledged_by"
        case resolvedAt = "resolved_at"
        case resolvedBy = "resolved_by"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

struct IncidentListResponse: Codable, Sendable { let incidents: [GardenIncident] }
struct IncidentActorRequest: Codable, Sendable { let actor: String }
struct IncidentResolutionRequest: Codable, Sendable {
    let outcome: IncidentResolution
    let actor: String
    let conclusion: String
}
