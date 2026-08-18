import Foundation

/// How strongly a diagnosis reply is allowed to be dressed. Only `.urgent`
/// unlocks red, and it also removes the mascot.
enum DiagnosisSeverity: String, FallbackDecodable {
    case finding
    case noConcern = "no_concern"
    case insufficient
    case urgent
    case unknown

    static let fallback = DiagnosisSeverity.unknown

    var isUrgent: Bool { self == .urgent }
}

/// One reply. The three fields are kept apart on purpose: what can be seen,
/// what it probably means, and what to do today are different claims.
struct DiagnosisTurn: Codable, Sendable, Hashable, Identifiable {
    var id: UUID = UUID()
    var severity: DiagnosisSeverity = .finding
    var observed: String
    var interpretation: String?
    var actionToday: String?
    var followUpPlan: String?
    var citation: String?
    var suggestedFollowUps: [String] = []
    var offeredActions: [DiagnosisOfferedAction] = []

    enum CodingKeys: String, CodingKey {
        case id
        case severity
        case observed
        case interpretation
        case actionToday = "action_today"
        case followUpPlan = "follow_up_plan"
        case citation
        case suggestedFollowUps = "suggested_follow_ups"
        case offeredActions = "offered_actions"
    }
}

/// A button the reply offers, which records an observation when tapped.
struct DiagnosisOfferedAction: Codable, Sendable, Hashable, Identifiable {
    var id: UUID = UUID()
    var title: String
    var recordsKind: ObservationKind?
    var note: String?

    enum CodingKeys: String, CodingKey {
        case id
        case title
        case recordsKind = "records_kind"
        case note
    }
}

/// One thing said in the conversation, by either side.
struct DiagnosisMessage: Sendable, Hashable, Identifiable {
    enum Speaker: Sendable, Hashable {
        case user
        case planty
    }

    let id: UUID
    let speaker: Speaker
    let text: String
    let turn: DiagnosisTurn?
    let sentAt: Date

    init(speaker: Speaker, text: String, turn: DiagnosisTurn? = nil, sentAt: Date = Date()) {
        id = UUID()
        self.speaker = speaker
        self.text = text
        self.turn = turn
        self.sentAt = sentAt
    }
}
