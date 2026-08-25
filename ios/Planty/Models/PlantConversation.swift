import Foundation

struct PlantConversationSummary: Decodable, Sendable, Hashable, Identifiable {
    let id: UUID
    let firstAsked: String
    let latestReply: String
    let turnCount: Int
    let startedAt: Date
    let updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case firstAsked = "first_asked"
        case latestReply = "latest_reply"
        case turnCount = "turn_count"
        case startedAt = "started_at"
        case updatedAt = "updated_at"
    }
}

struct PlantConversation: Decodable, Sendable, Hashable, Identifiable {
    let id: UUID
    let turns: [PlantConversationTurn]
}

struct PlantConversationTurn: Decodable, Sendable, Hashable, Identifiable {
    let id: UUID
    let conversationID: UUID
    let asked: String
    let reply: String
    let confidence: Double
    let lookedAt: String?
    let suggestedFollowUps: [String]
    let steps: [AnswerStep]
    let photoID: UUID?
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case conversationID = "conversation_id"
        case asked
        case reply
        case confidence
        case lookedAt = "looked_at"
        case suggestedFollowUps = "suggested_follow_ups"
        case steps
        case photoID = "photo_id"
        case createdAt = "created_at"
    }

    init(
        id: UUID,
        conversationID: UUID,
        asked: String,
        reply: String,
        confidence: Double,
        lookedAt: String?,
        suggestedFollowUps: [String],
        steps: [AnswerStep],
        photoID: UUID?,
        createdAt: Date
    ) {
        self.id = id
        self.conversationID = conversationID
        self.asked = asked
        self.reply = reply
        self.confidence = confidence
        self.lookedAt = lookedAt
        self.suggestedFollowUps = suggestedFollowUps
        self.steps = steps
        self.photoID = photoID
        self.createdAt = createdAt
    }

    init(from decoder: Decoder) throws {
        let box = try decoder.container(keyedBy: CodingKeys.self)
        id = try box.decode(UUID.self, forKey: .id)
        conversationID = try box.decode(UUID.self, forKey: .conversationID)
        asked = try box.decode(String.self, forKey: .asked)
        reply = try box.decode(String.self, forKey: .reply)
        confidence = try box.decodeIfPresent(Double.self, forKey: .confidence) ?? 0
        lookedAt = try box.decodeIfPresent(String.self, forKey: .lookedAt)
        suggestedFollowUps = try box.decodeIfPresent([String].self, forKey: .suggestedFollowUps) ?? []
        steps = try box.decodeIfPresent([AnswerStep].self, forKey: .steps) ?? []
        photoID = try box.decodeIfPresent(UUID.self, forKey: .photoID)
        createdAt = try box.decode(Date.self, forKey: .createdAt)
    }

    var answer: PlantAnswer {
        PlantAnswer(
            id: id,
            conversationID: conversationID,
            reply: reply,
            confidence: confidence,
            lookedAt: lookedAt,
            suggestedFollowUps: suggestedFollowUps,
            steps: steps
        )
    }
}

struct PlantConversationListResponse: Decodable, Sendable {
    let conversations: [PlantConversationSummary]
}
