import Foundation

/// How dangerous a plant is to one audience. `unknown` is not a gentler `safe`:
/// it means nobody looked, and every rendering of it has to say exactly that.
enum ToxicityRating: String, FallbackDecodable, CaseIterable {
    case unknown
    case safe
    case mild
    case moderate
    case severe

    static let fallback = ToxicityRating.unknown

    /// Unknown outranks safe on purpose, so the worst rating across a household
    /// can never come back "safe" while one audience was never checked.
    var severityOrder: Int {
        switch self {
        case .safe: 0
        case .unknown: 1
        case .mild: 2
        case .moderate: 3
        case .severe: 4
        }
    }

    /// Somebody established this rating. False for `unknown`, which is what
    /// keeps it out of every calm, filled, reassuring treatment.
    var isChecked: Bool { self != .unknown }

    /// Never "safe", never blank. This label is the whole distance between
    /// "nobody checked" and "probably fine".
    var label: String {
        switch self {
        case .unknown: "Not checked"
        case .safe: "Safe"
        case .mild: "Mild"
        case .moderate: "Moderate"
        case .severe: "Severe"
        }
    }

    var symbol: String {
        switch self {
        case .unknown: "questionmark.circle"
        case .safe: "checkmark.circle.fill"
        case .mild: "exclamationmark.circle.fill"
        case .moderate: "exclamationmark.triangle.fill"
        case .severe: "exclamationmark.octagon.fill"
        }
    }

    /// Lowercase fragment for the divergence sentence, which reads them in a
    /// row: "Severe for cats, mild for dogs and people."
    var clause: String {
        switch self {
        case .unknown: "not checked"
        case .safe: "safe"
        case .mild: "mild"
        case .moderate: "moderate"
        case .severe: "severe"
        }
    }

    func sentence(for audience: ToxicityAudience) -> String {
        switch self {
        case .unknown: "Nobody has checked this for \(audience.plural)."
        case .safe: "Recorded as safe for \(audience.plural)."
        case .mild: "Mildly toxic to \(audience.plural)."
        case .moderate: "Moderately toxic to \(audience.plural)."
        case .severe: "Severely toxic to \(audience.plural)."
        }
    }
}

/// Who is at risk. Three separate answers because they are genuinely different
/// animals, which is the entire reason this data is worth carrying.
enum ToxicityAudience: String, CaseIterable, Sendable, Hashable {
    case cats
    case dogs
    case people

    var plural: String { rawValue }

    var label: String {
        switch self {
        case .cats: "Cats"
        case .dogs: "Dogs"
        case .people: "People"
        }
    }

    var symbol: String {
        switch self {
        case .cats: "cat.fill"
        case .dogs: "dog.fill"
        case .people: "person.fill"
        }
    }
}

/// Where the grading came from. `derived` has to be said out loud: sources
/// publish toxic or not, so anything finer than that is Planty inferring.
enum ToxicityBasis: String, FallbackDecodable, CaseIterable {
    case source
    case derived
    case unknown

    static let fallback = ToxicityBasis.unknown
}

/// Species-level toxicity, as the service sends it. Everything past the three
/// ratings is optional, and a plant nobody looked up carries only unknowns.
struct Toxicity: Codable, Sendable, Hashable {
    var cats: ToxicityRating = .unknown
    var dogs: ToxicityRating = .unknown
    var people: ToxicityRating = .unknown
    var basis: ToxicityBasis = .unknown

    var identifiedAs: String?
    var principle: String?
    var signs: String?
    var parts: [String] = []
    var routes: [String] = []
    var notes: String?
    var firstAid: String?
    var source: String?
    var checkedAt: Date?

    enum CodingKeys: String, CodingKey {
        case cats
        case dogs
        case people
        case basis
        case identifiedAs = "identified_as"
        case principle
        case signs
        case parts
        case routes
        case notes
        case firstAid = "first_aid"
        case source
        case checkedAt = "checked_at"
    }
}

extension Toxicity {
    /// An absent rating decodes to `unknown`, never to nothing-and-so-fine.
    /// A rating the app has never heard of does the same rather than guessing.
    init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        cats = try container.decodeIfPresent(ToxicityRating.self, forKey: .cats) ?? .unknown
        dogs = try container.decodeIfPresent(ToxicityRating.self, forKey: .dogs) ?? .unknown
        people = try container.decodeIfPresent(ToxicityRating.self, forKey: .people) ?? .unknown
        basis = try container.decodeIfPresent(ToxicityBasis.self, forKey: .basis) ?? .unknown

        identifiedAs = try container.decodeIfPresent(String.self, forKey: .identifiedAs)
        principle = try container.decodeIfPresent(String.self, forKey: .principle)
        signs = try container.decodeIfPresent(String.self, forKey: .signs)
        parts = try container.decodeIfPresent([String].self, forKey: .parts) ?? []
        routes = try container.decodeIfPresent([String].self, forKey: .routes) ?? []
        notes = try container.decodeIfPresent(String.self, forKey: .notes)
        firstAid = try container.decodeIfPresent(String.self, forKey: .firstAid)
        source = try container.decodeIfPresent(String.self, forKey: .source)
        checkedAt = try container.decodeIfPresent(Date.self, forKey: .checkedAt)
    }
}

extension Toxicity {
    func rating(for audience: ToxicityAudience) -> ToxicityRating {
        switch audience {
        case .cats: cats
        case .dogs: dogs
        case .people: people
        }
    }

    var ratings: [(audience: ToxicityAudience, rating: ToxicityRating)] {
        ToxicityAudience.allCases.map { ($0, rating(for: $0)) }
    }

    /// The worst of the three, with unknown ranked above safe, so a card can
    /// never headline "safe" on the strength of the columns that were filled in.
    var worst: ToxicityRating {
        ratings.map(\.rating).max { $0.severityOrder < $1.severityOrder } ?? .unknown
    }

    /// The three do not agree. Rare, and precisely the case that kills animals:
    /// a lily is a sore stomach for a dog and renal failure for a cat.
    var diverges: Bool { Set(ratings.map(\.rating)).count > 1 }

    /// Nobody has looked this up at all.
    var isUnchecked: Bool {
        checkedAt == nil && ratings.allSatisfy { !$0.rating.isChecked }
    }

    /// Somebody looked and still could not say, which is a different sentence
    /// from nobody having looked.
    var isCheckedButUnresolved: Bool {
        checkedAt != nil && ratings.allSatisfy { !$0.rating.isChecked }
    }

    /// Planty graded this itself rather than repeating what the source said.
    var isDerived: Bool { basis == .derived }

    /// One line, worst first, so nobody has to diff three chips to find out
    /// that something in this pot can kill a cat.
    var headline: String {
        switch worst {
        case .severe: "Severely toxic"
        case .moderate: "Moderately toxic"
        case .mild: "Mildly toxic"
        case .unknown: "Not checked"
        case .safe: "No known toxicity"
        }
    }

    /// Spelled out in words when the audiences disagree, because a difference
    /// nobody notices is the same as no data at all.
    var divergenceSentence: String? {
        guard diverges else { return nil }
        let byRating = Dictionary(grouping: ratings, by: \.rating)
        let clauses = byRating.keys
            .sorted { $0.severityOrder > $1.severityOrder }
            .map { rating in
                let names = (byRating[rating] ?? []).map(\.audience.plural)
                return "\(rating.clause) for \(Self.listed(names))"
            }
        let sentence = clauses.joined(separator: ", ") + "."
        return sentence.prefix(1).uppercased() + sentence.dropFirst()
    }

    func checkedLine(now: Date = Date()) -> String? {
        guard let checkedAt else { return nil }
        return "Checked \(RelativeAge.phrase(since: checkedAt, now: now))."
    }

    private static func listed(_ names: [String]) -> String {
        switch names.count {
        case 0: ""
        case 1: names[0]
        case 2: "\(names[0]) and \(names[1])"
        default: "\(names.dropLast().joined(separator: ", ")) and \(names[names.count - 1])"
        }
    }
}
