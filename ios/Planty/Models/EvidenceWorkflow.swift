import Foundation

struct EvidenceReference: Codable, Sendable, Hashable, Identifiable {
    let plantID: UUID
    let kind: EvidenceKind
    let id: UUID
    let phase: EvidencePhase

    enum CodingKeys: String, CodingKey {
        case plantID = "plant_id"
        case kind
        case id
        case phase
    }
}

struct EvidenceExpectation: Codable, Sendable, Hashable {
    let plantID: UUID
    let kind: EvidenceKind
    let instruction: String

    enum CodingKeys: String, CodingKey {
        case plantID = "plant_id"
        case kind
        case instruction
    }
}

struct EvidenceExperiment: Codable, Sendable, Hashable {
    let title: String
    let hypothesis: String
    let variableKind: String
    let variableValue: String
    let holdConstantRules: [String]
    let successCriteria: [String]

    enum CodingKeys: String, CodingKey {
        case title
        case hypothesis
        case variableKind = "variable_kind"
        case variableValue = "variable_value"
        case holdConstantRules = "hold_constant_rules"
        case successCriteria = "success_criteria"
    }
}

struct EvidenceGuardrail: Codable, Sendable, Hashable {
    let reason: String
    let conflictingKinds: [ObservationKind]
    let redFlags: [String]

    enum CodingKeys: String, CodingKey {
        case reason
        case conflictingKinds = "conflicting_kinds"
        case redFlags = "red_flags"
    }
}

struct GuardrailOverride: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let windowID: UUID
    let plantID: UUID
    let kind: ObservationKind
    let reason: String
    let source: ObservationSource
    let actor: String?
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case windowID = "window_id"
        case plantID = "plant_id"
        case kind
        case reason
        case source
        case actor
        case createdAt = "created_at"
    }
}

struct EvidenceWindow: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let kind: EvidenceWindowKind
    let status: EvidenceWindowStatus
    let plantIDs: [UUID]
    let interventionKind: ObservationKind
    let interventionObservationID: UUID?
    let baseline: [EvidenceReference]
    let expected: [EvidenceExpectation]
    let review: [EvidenceReference]
    let earliestReviewAt: Date
    let latestReviewAt: Date
    let startedAt: Date?
    let readyAt: Date?
    let completedAt: Date?
    let outcome: EvidenceWindowOutcome?
    let conclusion: String?
    let confoundedAt: Date?
    let confoundReason: String?
    let proposedBy: ObservationSource
    let proposedActor: String?
    let guardrail: EvidenceGuardrail?
    let experiment: EvidenceExperiment?
    let overrides: [GuardrailOverride]
    let createdAt: Date
    let updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case id, kind, status, baseline, expected, review, outcome, conclusion, guardrail, experiment, overrides
        case plantIDs = "plant_ids"
        case interventionKind = "intervention_kind"
        case interventionObservationID = "intervention_observation_id"
        case earliestReviewAt = "earliest_review_at"
        case latestReviewAt = "latest_review_at"
        case startedAt = "started_at"
        case readyAt = "ready_at"
        case completedAt = "completed_at"
        case confoundedAt = "confounded_at"
        case confoundReason = "confound_reason"
        case proposedBy = "proposed_by"
        case proposedActor = "proposed_actor"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

struct EvidenceReferenceRequest: Codable, Sendable, Hashable {
    let plantID: UUID
    let kind: EvidenceKind
    let id: UUID

    enum CodingKeys: String, CodingKey {
        case plantID = "plant_id"
        case kind, id
    }
}

struct RecheckProposal: Codable, Sendable, Hashable {
    let interventionKind: ObservationKind
    let baseline: [EvidenceReferenceRequest]
    let expected: [EvidenceExpectation]
    let earliestReviewAt: Date
    let latestReviewAt: Date
    let actor: String

    enum CodingKeys: String, CodingKey {
        case interventionKind = "intervention_kind"
        case baseline, expected, actor
        case earliestReviewAt = "earliest_review_at"
        case latestReviewAt = "latest_review_at"
    }
}

struct ExperimentProposal: Codable, Sendable, Hashable {
    let plantIDs: [UUID]
    let interventionKind: ObservationKind
    let baseline: [EvidenceReferenceRequest]
    let expected: [EvidenceExpectation]
    let earliestReviewAt: Date
    let latestReviewAt: Date
    let actor: String
    let title: String
    let hypothesis: String
    let variableKind: String
    let variableValue: String
    let holdConstantRules: [String]
    let successCriteria: [String]

    enum CodingKeys: String, CodingKey {
        case plantIDs = "plant_ids"
        case interventionKind = "intervention_kind"
        case baseline, expected, actor, title, hypothesis
        case earliestReviewAt = "earliest_review_at"
        case latestReviewAt = "latest_review_at"
        case variableKind = "variable_kind"
        case variableValue = "variable_value"
        case holdConstantRules = "hold_constant_rules"
        case successCriteria = "success_criteria"
    }
}

struct EvidenceWindowStart: Codable, Sendable { let observationID: UUID; let actor: String
    enum CodingKeys: String, CodingKey { case observationID = "observation_id"; case actor }
}
struct EvidenceWindowReview: Codable, Sendable { let evidence: [EvidenceReferenceRequest] }
struct EvidenceWindowConclusion: Codable, Sendable { let outcome: EvidenceWindowOutcome; let conclusion: String; let actor: String }
struct EvidenceWindowCancellation: Codable, Sendable { let reason: String; let actor: String }
struct GuardrailOverrideRequest: Codable, Sendable {
    let plantID: UUID; let kind: ObservationKind; let reason: String; let actor: String
    enum CodingKeys: String, CodingKey { case plantID = "plant_id"; case kind, reason, actor }
}
struct EvidenceWindowListResponse: Codable, Sendable {
    let rechecks: [EvidenceWindow]?
    let guardrails: [EvidenceWindow]?
    let experiments: [EvidenceWindow]?
}

struct EvidenceCoverage: Codable, Sendable, Hashable, Identifiable {
    let plant: Plant
    let photoCount: Int
    let latestPhotoAt: Date?
    let sensorCount: Int
    let hasSoilSensor: Bool
    let soilCalibrated: Bool
    let botanicalKnown: Bool
    let toxicityChecked: Bool
    let healthEstablished: Bool
    let nextBestInput: String?
    let why: String?

    var id: UUID { plant.id }

    enum CodingKeys: String, CodingKey {
        case plant
        case photoCount = "photo_count"
        case latestPhotoAt = "latest_photo_at"
        case sensorCount = "sensor_count"
        case hasSoilSensor = "has_soil_sensor"
        case soilCalibrated = "soil_calibrated"
        case botanicalKnown = "botanical_known"
        case toxicityChecked = "toxicity_checked"
        case healthEstablished = "health_established"
        case nextBestInput = "next_best_input"
        case why
    }
}

struct EvidenceCoverageResponse: Codable, Sendable {
    let plants: [EvidenceCoverage]
    let count: Int
    let complete: Int
}
