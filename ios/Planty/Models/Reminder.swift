import Foundation

/// A standing intent to do something to a plant on a rhythm, for the jobs no
/// sensor confirms. Mirrors internal/plant.Reminder.
struct Reminder: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let plantID: UUID
    var kind: ObservationKind

    /// How often a day qualifies, and when on that day. Separate because a
    /// mushroom kit misted at 8 and again at 20 cannot be said in days alone.
    var everyDays: Int
    var atHours: [Int]

    var active: Bool
    var note: String?

    /// Filled in by the list, not by the record itself: whether this is owed
    /// right now is a question about observations, answered by the service.
    var lastDone: Date?
    var due: Bool?

    enum CodingKeys: String, CodingKey {
        case id
        case plantID = "plant_id"
        case kind
        case everyDays = "every_days"
        case atHours = "at_hours"
        case active
        case note
        case lastDone = "last_done"
        case due
    }
}

/// One owed occurrence of a recurring reminder. The scheduled time belongs in
/// the identity because morning and evening misting are two different jobs.
struct DueReminder: Codable, Sendable, Hashable, Identifiable {
    let reminder: Reminder
    let plant: Plant
    let lastDone: Date?
    let dueAt: Date

    var id: String { occurrenceID }
    var occurrenceID: String {
        "\(reminder.id.uuidString)|\(dueAt.timeIntervalSince1970)"
    }

    enum CodingKeys: String, CodingKey {
        case reminder
        case plant
        case lastDone = "last_done"
        case dueAt = "due_at"
    }
}

struct ReminderListResponse: Decodable, Sendable {
    let reminders: [Reminder]
    var count: Int?
}

/// What the app sends to set one. Omitting the schedule means daily at the
/// digest hour, which is what "remind me to water this" usually means.
struct NewReminder: Encodable, Sendable {
    var kind: ObservationKind
    var everyDays: Int
    var atHours: [Int]
    var active: Bool
    var note: String?

    enum CodingKeys: String, CodingKey {
        case kind
        case everyDays = "every_days"
        case atHours = "at_hours"
        case active
        case note
    }
}

extension Reminder {
    /// The kinds worth nagging about: things a person does on a rhythm. There
    /// is no reminder to notice a symptom or to have a plant die.
    static let remindable: [ObservationKind] = [
        .watered, .misted, .fertilized, .pruned, .repotted
    ]

    var schedule: String {
        let times = atHours.sorted().map(Self.hourLabel).joined(separator: " and ")
        if everyDays == 1 { return "Every day at \(times)" }
        return "Every \(everyDays) days at \(times)"
    }

    static func hourLabel(_ hour: Int) -> String {
        var components = DateComponents()
        components.hour = hour
        components.minute = 0
        let calendar = Calendar.current
        guard let date = calendar.date(from: components) else { return "\(hour):00" }

        let formatter = DateFormatter()
        formatter.locale = .current
        formatter.setLocalizedDateFormatFromTemplate("j")
        return formatter.string(from: date)
    }
}

extension DueReminder {
    var completionLabel: String { reminder.kind.completionLabel }

    var dueLine: String {
        "Due at \(dueAt.formatted(date: .omitted, time: .shortened))"
    }
}

extension ObservationKind {
    /// What to do, not what was recorded: a reminder is an instruction, and
    /// "watered" is not one.
    var instruction: String {
        switch self {
        case .watered: "Water it"
        case .misted: "Mist it"
        case .fertilized: "Feed it"
        case .pruned: "Prune it"
        case .repotted: "Repot it"
        default: rawValue.capitalized
        }
    }

    /// The confirmation button says exactly what will be written to history.
    var completionLabel: String {
        switch self {
        case .watered: "I watered it"
        case .misted: "I misted it"
        case .fertilized: "I fed it"
        case .pruned: "I pruned it"
        case .repotted: "I repotted it"
        default: "I did it"
        }
    }
}
