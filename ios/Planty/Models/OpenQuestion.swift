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
    var status: QuestionStatus? = nil
    var answer: String? = nil
    var answeredAt: Date? = nil

    enum CodingKeys: String, CodingKey {
        case id
        case plantID = "plant_id"
        case askedOf = "asked_of"
        case question
        case why
        case createdAt = "created_at"
        case status
        case answer
        case answeredAt = "answered_at"
    }

    /// "self" is the operator, and naming them in their own app reads oddly.
    var isForSomebodyElse: Bool { askedOf != "self" && !askedOf.isEmpty }

    var audience: String { isForSomebodyElse ? askedOf : "you" }
}

enum QuestionStatus: String, Codable, Sendable, Hashable, CaseIterable {
    case open
    case answered
    case dropped
}

struct QuestionListResponse: Decodable, Sendable {
    let questions: [OpenQuestion]
    var count: Int?
    var asText: String?

    enum CodingKeys: String, CodingKey {
        case questions
        case count
        case asText = "as_text"
    }
}

struct NewOpenQuestion: Codable, Sendable, Hashable {
    var plantID: UUID? = nil
    var askedOf: String? = nil
    var question: String
    var why: String? = nil

    enum CodingKeys: String, CodingKey {
        case plantID = "plant_id"
        case askedOf = "asked_of"
        case question
        case why
    }
}

struct NewAwayPeriod: Codable, Sendable, Hashable {
    var startsAt: Date
    var endsAt: Date
    var backupContact: String? = nil
    var backupNotify: String? = nil
    var note: String? = nil

    enum CodingKeys: String, CodingKey {
        case startsAt = "starts_at"
        case endsAt = "ends_at"
        case backupContact = "backup_contact"
        case backupNotify = "backup_notify"
        case note
    }
}

struct AwayPeriod: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let startsAt: Date
    let endsAt: Date
    var backupContact: String?
    var backupNotify: String?
    var note: String?
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case startsAt = "starts_at"
        case endsAt = "ends_at"
        case backupContact = "backup_contact"
        case backupNotify = "backup_notify"
        case note
        case createdAt = "created_at"
    }
}

struct ColdWatch: Codable, Sendable, Hashable {
    let forecastLowF: Double
    let plants: [Plant]
    var count: Int?

    enum CodingKeys: String, CodingKey {
        case forecastLowF = "forecast_low_f"
        case plants
        case count
    }
}

/// What POST /v1/questions/{id}/answer takes.
struct QuestionAnswer: Encodable, Sendable, Hashable {
    var answer: String
}
