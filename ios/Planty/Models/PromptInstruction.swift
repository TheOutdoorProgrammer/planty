import Foundation

/// App-facing form of the wire instruction. The generated OpenAPI object keeps
/// `job` and `updated_at` as strings; this domain model gives both real types.
struct PromptInstructionSetting: Codable, Sendable, Hashable, Identifiable {
    let job: AIJob
    var instructions: String
    var updatedAt: Date?

    var id: AIJob { job }
    var hasOverlay: Bool { !instructions.cleaned.isEmpty }

    enum CodingKeys: String, CodingKey {
        case job
        case instructions
        case updatedAt = "updated_at"
    }
}

struct PromptInstructionListResponse: Codable, Sendable {
    let instructions: [PromptInstructionSetting]
}

struct PromptOverlayDraft: Equatable, Sendable {
    static let maximumBytes = 12_000
    var instructions: String

    var cleaned: String { instructions.cleaned }
    var isValid: Bool {
        !cleaned.isEmpty && cleaned.lengthOfBytes(using: .utf8) <= Self.maximumBytes
    }
}
