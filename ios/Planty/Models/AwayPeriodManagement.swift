import Foundation

struct AwayPeriodListResponse: Decodable, Sendable {
    let awayPeriods: [AwayPeriod]
    var count: Int?

    enum CodingKeys: String, CodingKey {
        case awayPeriods = "away_periods"
        case count
    }
}

/// PATCH sends every editable field. Empty strings are intentional: unlike an
/// omitted optional value, they tell the service to clear an existing backup
/// contact, notify target, or note.
struct AwayPeriodUpdate: Encodable, Sendable, Hashable {
    let startsAt: Date
    let endsAt: Date
    let backupContact: String
    let backupNotify: String
    let note: String

    init(_ draft: NewAwayPeriod) {
        startsAt = draft.startsAt
        endsAt = draft.endsAt
        backupContact = draft.backupContact ?? ""
        backupNotify = draft.backupNotify ?? ""
        note = draft.note ?? ""
    }

    enum CodingKeys: String, CodingKey {
        case startsAt = "starts_at"
        case endsAt = "ends_at"
        case backupContact = "backup_contact"
        case backupNotify = "backup_notify"
        case note
    }
}

extension AwayPeriod {
    func covers(_ date: Date) -> Bool {
        date >= startsAt && date < endsAt
    }

    func Covers(_ date: Date) -> Bool {
        covers(date)
    }
}
