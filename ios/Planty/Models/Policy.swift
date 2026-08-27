import Foundation

struct OPAPolicy: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    var name: String
    var description: String
    var source: String
    var mode: PolicyMode
    var enabled: Bool
    let version: Int
    let createdAt: Date
    let updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case id, name, description, source, mode, enabled, version
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

struct PolicyDraft: Codable, Sendable, Equatable {
    static let maximumBytes = 65_536

    var name: String
    var description: String
    var source: String
    var mode: PolicyMode
    var enabled: Bool

    var isValid: Bool {
        !name.cleaned.isEmpty && !source.cleaned.isEmpty &&
            source.lengthOfBytes(using: .utf8) <= Self.maximumBytes && mode != .unknown
    }

    static func new(reference: PolicyReference?) -> PolicyDraft {
        PolicyDraft(
            name: "Plant care rules",
            description: "Turns current plant evidence into clear care signals.",
            source: reference?.example ?? Self.fallbackExample,
            mode: .advisory,
            enabled: false
        )
    }

    static let fallbackExample = """
        package planty

        default decision := {
          "summary": "No policy action needed.",
          "signals": [],
          "notifications": [],
          "fan_runs": [],
          "agent": {"facts": [], "guidance": [], "deny_actions": []},
        }
        """
}

struct PolicyListResponse: Codable, Sendable {
    let policies: [OPAPolicy]
}

struct PolicyPreviewRequest: Codable, Sendable {
    let name: String
    let description: String
    let source: String
    let mode: PolicyMode
    let enabled: Bool
    let plantSlug: String

    enum CodingKeys: String, CodingKey {
        case name, description, source, mode, enabled
        case plantSlug = "plant_slug"
    }
}

struct PolicyEvaluationRequest: Codable, Sendable {
    let plantSlug: String

    enum CodingKeys: String, CodingKey { case plantSlug = "plant_slug" }
}

struct PolicyPreview: Codable, Sendable {
    let decision: PolicyDecision
    let durationMS: Double

    enum CodingKeys: String, CodingKey {
        case decision
        case durationMS = "duration_ms"
    }
}

struct PolicyDecision: Codable, Sendable, Hashable {
    let summary: String
    let signals: [PolicySignal]
    let health: PolicyHealthAdjustment?
    let notifications: [PolicyNotification]
    let fanRuns: [PolicyFanRun]
    let agent: PolicyAgentGuidance

    enum CodingKeys: String, CodingKey {
        case summary, signals, health, notifications, agent
        case fanRuns = "fan_runs"
    }
}

struct PolicySignal: Codable, Sendable, Hashable, Identifiable {
    let kind: PolicySignalKind
    let active: Bool
    let severity: PolicySeverity
    let reason: String
    let confidence: Double?

    var id: String { "\(kind.rawValue):\(reason)" }
}

struct PolicyHealthAdjustment: Codable, Sendable, Hashable {
    let delta: Double
    let reason: String
}

struct PolicyNotification: Codable, Sendable, Hashable, Identifiable {
    let title: String
    let body: String
    let priority: PolicySeverity

    var id: String { "\(title):\(body)" }
}

struct PolicyFanRun: Codable, Sendable, Hashable, Identifiable {
    let actuatorID: UUID
    let durationSeconds: Int
    let reason: String

    var id: UUID { actuatorID }

    enum CodingKeys: String, CodingKey {
        case actuatorID = "actuator_id"
        case durationSeconds = "duration_seconds"
        case reason
    }
}

struct PolicyAgentGuidance: Codable, Sendable, Hashable {
    let facts: [String]
    let guidance: [String]
    let denyActions: [String]

    enum CodingKeys: String, CodingKey {
        case facts, guidance
        case denyActions = "deny_actions"
    }
}

struct PolicyEvaluation: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let policyID: UUID
    let policyVersion: Int
    let policyMode: PolicyMode
    let plantID: UUID
    let trigger: PolicyTrigger
    let decision: PolicyDecision
    let durationMS: Double
    let outcome: String
    let error: String?
    let enforced: [String]
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id, trigger, decision, outcome, error, enforced
        case policyID = "policy_id"
        case policyVersion = "policy_version"
        case policyMode = "policy_mode"
        case plantID = "plant_id"
        case durationMS = "duration_ms"
        case createdAt = "created_at"
    }
}

struct PolicyEvaluationListResponse: Codable, Sendable {
    let evaluations: [PolicyEvaluation]
}

struct PolicyReference: Codable, Sendable, Hashable {
    let inputVersion: String
    let entrypoint: String
    let sections: [PolicyReferenceSection]
    let output: [PolicyReferenceField]
    let example: String
    let safety: [String]

    enum CodingKeys: String, CodingKey {
        case entrypoint, sections, output, example, safety
        case inputVersion = "input_version"
    }
}

struct PolicyReferenceSection: Codable, Sendable, Hashable, Identifiable {
    let title: String
    let fields: [PolicyReferenceField]
    var id: String { title }
}

struct PolicyReferenceField: Codable, Sendable, Hashable, Identifiable {
    let path: String
    let type: String
    let description: String
    var id: String { path }
}
