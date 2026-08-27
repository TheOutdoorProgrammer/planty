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
        package planty.v1

        incident if {
          input.plant.is_sick
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
    let result: PolicyResult
    let durationMS: Double

    enum CodingKeys: String, CodingKey {
        case result
        case durationMS = "duration_ms"
    }
}

struct PolicyResult: Codable, Sendable, Hashable {
    let rules: [PolicyRule]
    let health: PolicyHealthAdjustment?
    let notifications: [PolicyNotification]
    let fanRuns: [PolicyFanRun]
    let agent: PolicyAgentGuidance

    enum CodingKeys: String, CodingKey {
        case rules, health, notifications, agent
        case fanRuns = "fan_runs"
    }
}

struct PolicyRule: Codable, Sendable, Hashable, Identifiable {
    let name: String
    let active: Bool
    let value: JSONValue

    var id: String { name }
}

enum JSONValue: Codable, Sendable, Hashable {
    case null
    case bool(Bool)
    case number(Double)
    case string(String)
    case array([JSONValue])
    case object([String: JSONValue])

    init(from decoder: Decoder) throws {
        let container = try decoder.singleValueContainer()
        if container.decodeNil() {
            self = .null
        } else if let value = try? container.decode(Bool.self) {
            self = .bool(value)
        } else if let value = try? container.decode(Double.self) {
            self = .number(value)
        } else if let value = try? container.decode(String.self) {
            self = .string(value)
        } else if let value = try? container.decode([JSONValue].self) {
            self = .array(value)
        } else {
            self = .object(try container.decode([String: JSONValue].self))
        }
    }

    func encode(to encoder: Encoder) throws {
        var container = encoder.singleValueContainer()
        switch self {
        case .null: try container.encodeNil()
        case let .bool(value): try container.encode(value)
        case let .number(value): try container.encode(value)
        case let .string(value): try container.encode(value)
        case let .array(value): try container.encode(value)
        case let .object(value): try container.encode(value)
        }
    }

    var displayValue: String {
        switch self {
        case .null: "null"
        case let .bool(value): value ? "true" : "false"
        case let .number(value): value.formatted()
        case let .string(value): value
        case let .array(value): "[\(value.map(\.displayValue).joined(separator: ", "))]"
        case let .object(value):
            "{\(value.keys.sorted().map { "\($0): \(value[$0]!.displayValue)" }.joined(separator: ", "))}"
        }
    }
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
    let result: PolicyResult
    let durationMS: Double
    let outcome: String
    let error: String?
    let enforced: [String]
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id, trigger, result, outcome, error, enforced
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
