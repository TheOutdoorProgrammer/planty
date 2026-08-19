import Foundation

/// The answer to "what should I do right now". Mirrors internal/plant.Digest.
struct Digest: Codable, Sendable, Hashable {
    let date: Date
    private(set) var entries: [DigestEntry]

    /// checked and staleSince are what let the app tell "nothing needs doing"
    /// apart from "no fresh data". They must never render the same.
    let checked: Int
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
        case checked
        case staleSince = "stale_since"
        case neverRun = "never_run"
        case openQuestions = "open_questions"
    }

    /// Nothing to do, the data behind that claim is fresh, and something has
    /// actually been checked.
    var isAllClear: Bool { entries.isEmpty && staleSince == nil && !neverRun }

    init(
        date: Date,
        entries: [DigestEntry],
        checked: Int,
        staleSince: Date? = nil,
        neverRun: Bool = false,
        openQuestions: [OpenQuestion] = []
    ) {
        self.openQuestions = openQuestions
        self.date = date
        self.entries = entries
        self.checked = checked
        self.staleSince = staleSince
        self.neverRun = neverRun
    }

    init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        date = try container.decode(Date.self, forKey: .date)
        checked = try container.decode(Int.self, forKey: .checked)
        staleSince = try container.decodeIfPresent(Date.self, forKey: .staleSince)
        neverRun = try container.decodeIfPresent(Bool.self, forKey: .neverRun) ?? false

        // An empty list is null on some encoders, and refusing to decode the
        // whole digest over that took the Today tab down entirely.
        entries = try container.decodeIfPresent([DigestEntry].self, forKey: .entries) ?? []
        openQuestions = try container.decodeIfPresent(
            [OpenQuestion].self, forKey: .openQuestions) ?? []
    }

    /// Drops entries the user already settled, without touching `checked`:
    /// the freshness count has to keep describing what the service did.
    func hiding(verdictIDs: Set<UUID>) -> Digest {
        var copy = self
        copy.entries = entries.filter { !verdictIDs.contains($0.verdict.id) }
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
}

struct DigestEntry: Codable, Sendable, Hashable, Identifiable {
    let plant: Plant
    let verdict: Verdict
    let risk: Int

    var id: UUID { verdict.id }
}

/// The service's error body: {"error": "..."}.
struct APIErrorBody: Decodable, Sendable {
    let error: String
}
