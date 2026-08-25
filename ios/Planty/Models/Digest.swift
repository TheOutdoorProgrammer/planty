import Foundation

/// The answer to "what should I do right now". Mirrors internal/plant.Digest
/// and adds scheduled reminder occurrences that are due alongside judgments.
struct Digest: Codable, Sendable, Hashable {
    let date: Date
    private(set) var entries: [DigestEntry]
    private(set) var dueReminders: [DueReminder]

    /// Completeness belongs to the persisted garden-wide judgment run. A
    /// partial run must never look identical to a complete all-clear.
    let checked: Int
    let expected: Int
    let failed: Int
    let failures: [JudgmentFailure]
    let runComplete: Bool
    var staleSince: Date?

    /// Nothing has ever been judged. Distinct from "judged, nothing to do",
    /// and the difference is the whole point: a garden nobody has looked at
    /// must never render as calm.
    var neverRun = false

    /// Waiting on a person rather than on a plant. Absent from older services,
    /// so it defaults empty rather than failing the whole digest.
    var openQuestions: [OpenQuestion] = []

    enum CodingKeys: String, CodingKey {
        case date
        case entries
        case dueReminders = "due_reminders"
        case checked
        case expected
        case failed
        case failures
        case runComplete = "run_complete"
        case staleSince = "stale_since"
        case neverRun = "never_run"
        case openQuestions = "open_questions"
    }

    var isAllClear: Bool {
        entries.isEmpty && dueReminders.isEmpty && staleSince == nil && !neverRun &&
            runComplete && failed == 0 && checked == expected
    }

    init(
        date: Date,
        entries: [DigestEntry],
        dueReminders: [DueReminder] = [],
        checked: Int,
        expected: Int? = nil,
        failed: Int = 0,
        failures: [JudgmentFailure] = [],
        runComplete: Bool = true,
        staleSince: Date? = nil,
        neverRun: Bool = false,
        openQuestions: [OpenQuestion] = []
    ) {
        self.openQuestions = openQuestions
        self.date = date
        self.entries = entries
        self.dueReminders = dueReminders
        self.checked = checked
        self.expected = expected ?? checked
        self.failed = failed
        self.failures = failures
        self.runComplete = runComplete
        self.staleSince = staleSince
        self.neverRun = neverRun
    }

    init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        date = try container.decode(Date.self, forKey: .date)
        checked = try container.decode(Int.self, forKey: .checked)
        expected = try container.decodeIfPresent(Int.self, forKey: .expected) ?? checked
        failed = try container.decodeIfPresent(Int.self, forKey: .failed) ?? 0
        failures = try container.decodeIfPresent([JudgmentFailure].self, forKey: .failures) ?? []
        runComplete = try container.decodeIfPresent(Bool.self, forKey: .runComplete) ?? true
        staleSince = try container.decodeIfPresent(Date.self, forKey: .staleSince)
        neverRun = try container.decodeIfPresent(Bool.self, forKey: .neverRun) ?? false

        // An empty list is null on some encoders, and refusing to decode the
        // whole digest over that took the Today tab down entirely.
        entries = try container.decodeIfPresent([DigestEntry].self, forKey: .entries) ?? []
        dueReminders = try container.decodeIfPresent(
            [DueReminder].self, forKey: .dueReminders) ?? []
        openQuestions = try container.decodeIfPresent(
            [OpenQuestion].self, forKey: .openQuestions) ?? []
    }

    /// Drops work the user already settled, without touching `checked`: the
    /// freshness count has to keep describing what the service did. Reminder
    /// identities include the due slot, so finishing morning misting cannot
    /// suppress the evening occurrence.
    func hiding(
        verdictIDs: Set<UUID>,
        reminderOccurrenceIDs: Set<String> = []
    ) -> Digest {
        var copy = self
        copy.entries = entries.filter { !verdictIDs.contains($0.verdict.id) }
        copy.dueReminders = dueReminders.filter {
            !reminderOccurrenceIDs.contains($0.occurrenceID)
        }
        return copy
    }

    /// `/v1/today` carries the judged/reminded plant record, while
    /// `/v1/plants` carries the newest photo URL. Today already loads both, so
    /// merge only photo metadata instead of adding another request.
    func keepingPhotos(from plants: [Plant]) -> Digest {
        let byID = Dictionary(uniqueKeysWithValues: plants.map { ($0.id, $0) })
        var copy = self
        copy.entries = entries.map { entry in
            guard let listed = byID[entry.plant.id] else { return entry }
            return DigestEntry(
                plant: entry.plant.keepingPhoto(from: listed),
                verdict: entry.verdict,
                risk: entry.risk
            )
        }
        copy.dueReminders = dueReminders.map { occurrence in
            guard let listed = byID[occurrence.plant.id] else { return occurrence }
            return DueReminder(
                reminder: occurrence.reminder,
                plant: occurrence.plant.keepingPhoto(from: listed),
                lastDone: occurrence.lastDone,
                dueAt: occurrence.dueAt
            )
        }
        return copy
    }

    /// Friend-owned first, then by the service's own neglect risk. Ownership
    /// changes the order and nothing else about how the card looks.
    var sortedEntries: [DigestEntry] {
        entries.sorted { lhs, rhs in
            if lhs.plant.isFriends != rhs.plant.isFriends {
                return lhs.plant.isFriends
            }
            if lhs.risk != rhs.risk {
                return lhs.risk > rhs.risk
            }
            return lhs.plant.commonName.localizedCompare(rhs.plant.commonName) == .orderedAscending
        }
    }

    /// Reminders are ordered by the slot first, with friend-owned plants first
    /// inside a slot so care promised to somebody else is hardest to overlook.
    var sortedDueReminders: [DueReminder] {
        dueReminders.sorted { lhs, rhs in
            if lhs.dueAt != rhs.dueAt { return lhs.dueAt < rhs.dueAt }
            if lhs.plant.isFriends != rhs.plant.isFriends {
                return lhs.plant.isFriends
            }
            return lhs.plant.commonName.localizedCompare(rhs.plant.commonName) == .orderedAscending
        }
    }
}

struct JudgmentFailure: Codable, Sendable, Hashable, Identifiable {
    let runID: UUID
    let plant: Plant
    let attempts: Int
    let model: String?
    let originalError: String?
    let finalError: String?
    let updatedAt: Date

    var id: UUID { plant.id }

    enum CodingKeys: String, CodingKey {
        case runID = "run_id"
        case plant
        case attempts
        case model
        case originalError = "original_error"
        case finalError = "final_error"
        case updatedAt = "updated_at"
    }
}

struct DigestEntry: Codable, Sendable, Hashable, Identifiable {
    let plant: Plant
    let verdict: Verdict
    let risk: Int

    var id: UUID { verdict.id }
}
