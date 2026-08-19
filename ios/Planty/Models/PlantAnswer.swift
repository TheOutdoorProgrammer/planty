import Foundation

/// An answer to a question about one plant, read from its record rather than
/// from a photograph. Mirrors POST /v1/plants/{slug}/ask.
struct PlantAnswer: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let conversationID: UUID
    let reply: String
    let confidence: Double

    /// Which photographs it chose to open, empty when it needed none. Worth
    /// showing: the whole point is that looking is optional, and this is the
    /// only way to tell whether it did.
    let lookedAt: String?
    let suggestedFollowUps: [String]

    enum CodingKeys: String, CodingKey {
        case id
        case conversationID = "conversation_id"
        case reply
        case confidence
        case lookedAt = "looked_at"
        case suggestedFollowUps = "suggested_follow_ups"
    }

    var didOpenAPhotograph: Bool {
        guard let lookedAt else { return false }
        return !lookedAt.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }
}

/// What the app sends to ask something. No photograph: the record is the
/// subject, and requiring a new picture to ask anything is the thing this
/// endpoint exists to avoid.
struct PlantQuestion: Encodable, Sendable {
    var message: String
    var conversationID: UUID?

    enum CodingKeys: String, CodingKey {
        case message
        case conversationID = "conversation_id"
    }
}
