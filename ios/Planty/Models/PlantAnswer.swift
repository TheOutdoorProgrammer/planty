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

/// What the app sends to ask something. The record is the subject, so a photo
/// is optional: it is there for the turn where the model asks to see one.
struct PlantQuestion: Encodable, Sendable, Hashable {
    var message: String

    /// Base64 JPEG on the wire, which is what `Data` encodes to by default.
    var photo: Data?
    var conversationID: UUID?

    enum CodingKeys: String, CodingKey {
        case message
        case photo
        case conversationID = "conversation_id"
    }
}

/// POST /v1/ask. A question about anything at all: no plant is named, none is
/// created, and no plant's timeline is touched.
struct ScratchQuestion: Encodable, Sendable, Hashable {
    /// Both optional, at least one required. A photo on its own is a question.
    var message: String?
    var photo: Data?
    var conversationID: UUID?

    enum CodingKeys: String, CodingKey {
        case message
        case photo
        case conversationID = "conversation_id"
    }
}

/// What POST /v1/ask answers with. Flatter than PlantAnswer because there is no
/// record behind it to cite, look at, or suggest follow-ups from.
struct ScratchAnswer: Decodable, Sendable, Hashable, Identifiable {
    let id: UUID
    let conversationID: UUID
    let answer: String

    enum CodingKeys: String, CodingKey {
        case id
        case conversationID = "conversation_id"
        case answer
    }
}
