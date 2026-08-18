import Foundation

/// The answer to "what should I do right now". Mirrors internal/plant.Digest.
struct Digest: Codable, Sendable, Hashable {
    let date: Date
    private(set) var entries: [DigestEntry]

    /// checked and staleSince are what let the app tell "nothing needs doing"
    /// apart from "no fresh data". They must never render the same.
    let checked: Int
    var staleSince: Date?

    enum CodingKeys: String, CodingKey {
        case date
        case entries
        case checked
        case staleSince = "stale_since"
    }

    /// Nothing to do AND the data behind that claim is actually fresh.
    var isAllClear: Bool { entries.isEmpty && staleSince == nil }

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
