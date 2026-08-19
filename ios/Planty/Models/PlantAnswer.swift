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

    /// What it did to reach the answer: commands run, pages fetched, thinking
    /// in between. Absent on older answers, which is why it defaults empty.
    let steps: [AnswerStep]

    enum CodingKeys: String, CodingKey {
        case id
        case conversationID = "conversation_id"
        case reply
        case confidence
        case lookedAt = "looked_at"
        case suggestedFollowUps = "suggested_follow_ups"
        case steps
    }

    init(
        id: UUID,
        conversationID: UUID,
        reply: String,
        confidence: Double,
        lookedAt: String?,
        suggestedFollowUps: [String],
        steps: [AnswerStep] = []
    ) {
        self.id = id
        self.conversationID = conversationID
        self.reply = reply
        self.confidence = confidence
        self.lookedAt = lookedAt
        self.suggestedFollowUps = suggestedFollowUps
        self.steps = steps
    }

    init(from decoder: Decoder) throws {
        let box = try decoder.container(keyedBy: CodingKeys.self)
        id = try box.decode(UUID.self, forKey: .id)
        conversationID = try box.decode(UUID.self, forKey: .conversationID)
        reply = try box.decode(String.self, forKey: .reply)
        confidence = try box.decodeIfPresent(Double.self, forKey: .confidence) ?? 0
        lookedAt = try box.decodeIfPresent(String.self, forKey: .lookedAt)
        suggestedFollowUps = try box.decodeIfPresent([String].self, forKey: .suggestedFollowUps) ?? []
        steps = try box.decodeIfPresent([AnswerStep].self, forKey: .steps) ?? []
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
/// created, and no plant's timeline is touched. Answered by `PlantAnswer`, the
/// same shape the plant-scoped endpoint returns.
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
