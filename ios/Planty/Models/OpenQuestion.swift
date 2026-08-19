import Foundation

/// Something Planty wants settled that only a person can settle, queued for
/// somebody who was not in the conversation: usually the friend whose plants
/// these are.
struct OpenQuestion: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let plantID: UUID?
    let askedOf: String
    let question: String
    let why: String?
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case plantID = "plant_id"
        case askedOf = "asked_of"
        case question
        case why
        case createdAt = "created_at"
    }

    /// "self" is the operator, and naming them in their own app reads oddly.
    var isForSomebodyElse: Bool { askedOf != "self" && !askedOf.isEmpty }

    var audience: String { isForSomebodyElse ? askedOf : "you" }
}

/// What POST /v1/questions/{id}/answer takes.
struct QuestionAnswer: Encodable, Sendable, Hashable {
    var answer: String
}
