import Foundation

/// How old the evidence behind a digest is allowed to get before the app stops
/// being willing to say everything is fine.
struct FreshnessPolicy: Sendable, Equatable {
    /// The judgment run is daily, so one missed morning is still explainable
    /// and two is not. DESIGN-NOTES.md leaves the number open; this is the call.
    var maxAge: TimeInterval = 36 * 60 * 60

    static let standard = FreshnessPolicy()
}

/// Why the app does or does not trust a digest. `stale` is never rendered with
/// calm chrome, which is the single rule this whole screen exists to honour.
enum Freshness: Sendable, Equatable {
    case fresh
    case stale(since: Date, reason: StaleReason)

    var isFresh: Bool { self == .fresh }

    var since: Date? {
        if case .stale(let since, _) = self { return since }
        return nil
    }
}

enum StaleReason: Sendable, Equatable {
    /// The service itself said its evidence stops here.
    case serviceReported
    /// The digest is older than the policy allows.
    case tooOld
    /// A run happened but looked at nothing, while plants exist.
    case checkedNothing
    /// The latest garden-wide run did not successfully cover every plant.
    case incompleteCheck

    var sentence: String {
        switch self {
        case .serviceReported, .tooOld:
            "That does not mean anything is wrong; it means Planty cannot honestly say everything is fine."
        case .checkedNothing:
            "The last run finished without reading any plant, so there is nothing behind a calm answer."
        case .incompleteCheck:
            "The latest run did not successfully check every plant, so Planty will not call the garden all clear."
        }
    }
}

extension Digest {
    /// Completeness outranks ordinary age: a check that failed five minutes ago
    /// is not fresh just because its timestamp is recent.
    func freshness(
        now: Date,
        knownPlantCount: Int?,
        policy: FreshnessPolicy = .standard
    ) -> Freshness {
        if !runComplete || failed > 0 || checked != expected {
            return .stale(since: date, reason: .incompleteCheck)
        }
        if let staleSince {
            return .stale(since: staleSince, reason: .serviceReported)
        }
        if now.timeIntervalSince(date) > policy.maxAge {
            return .stale(since: date, reason: .tooOld)
        }
        if checked == 0, let knownPlantCount, knownPlantCount > 0 {
            return .stale(since: date, reason: .checkedNothing)
        }
        return .fresh
    }
}

/// Wording for how long ago something happened, used everywhere the app has to
/// name freshness out loud.
enum RelativeAge {
    /// Uses RelativeDateTimeFormatter, not `.formatted(.relative:)`, because
    /// the latter has no reference date and silently uses the system clock.
    static func phrase(since: Date, now: Date) -> String {
        if now.timeIntervalSince(since) < 90 * 60 {
            return "less than an hour ago"
        }
        let formatter = RelativeDateTimeFormatter()
        formatter.unitsStyle = .full
        formatter.dateTimeStyle = .named
        return formatter.localizedString(for: since, relativeTo: now)
    }

    /// "yesterday at 8:01 AM", for the loading row that keeps old data visible.
    static func dayAndTime(_ date: Date, now: Date, calendar: Calendar = .current) -> String {
        let time = date.formatted(date: .omitted, time: .shortened)
        if calendar.isDate(date, inSameDayAs: now) {
            return "today at \(time)"
        }
        if let yesterday = calendar.date(byAdding: .day, value: -1, to: now),
           calendar.isDate(date, inSameDayAs: yesterday) {
            return "yesterday at \(time)"
        }
        return "\(date.formatted(.dateTime.month(.wide).day())) at \(time)"
    }
}
