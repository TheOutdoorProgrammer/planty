import Foundation

/// Everything Today can be, decided in one pure function so the calm-versus-
/// stale rule can be tested without a screen, a network or a clock.
enum TodayPresentation: Sendable, Equatable {
    case unconfigured
    case loadingCold
    case loadingWarm(previous: Digest, checkedAt: Date)
    case emptySetup
    case calm(CalmSummary)
    case actions(ActionSummary)
    case stale(StaleSummary)
    case failed(error: PlantyError, cached: Digest?)

    /// The mascot belongs to reassurance and gentle guidance only. It never
    /// appears next to an alert, a stale warning or a failure.
    var showsMascot: Bool {
        switch self {
        case .calm, .emptySetup, .loadingCold, .loadingWarm: true
        case .unconfigured, .actions, .stale, .failed: false
        }
    }
}

struct CalmSummary: Sendable, Equatable {
    let checked: Int
    let updatedAt: Date
    /// The last card was cleared in this session, so the copy acknowledges it
    /// instead of pretending there was never anything to do.
    var justFinished = false

    var headline: String { justFinished ? "That's it." : "You're done." }

    var subhead: String {
        justFinished ? "Everything else is okay." : "Nothing needs you right now."
    }

    var freshnessLine: String {
        let plants = checked == 1 ? "1 plant checked" : "\(checked) plants checked"
        return "\(plants) · Updated at \(updatedAt.formatted(date: .omitted, time: .shortened))"
    }

    var reassuranceTitle: String { "All quiet in the greenhouse" }
    var reassuranceBody: String { "The next automatic check is tomorrow morning." }
}

struct ActionSummary: Sendable, Equatable {
    /// Expanded cards, friend-owned first. Never more than three.
    let featured: [DigestEntry]
    let deferred: [DigestEntry]
    let checked: Int
    let updatedAt: Date

    static let maxFeatured = 3

    var headline: String {
        switch featured.count + deferred.count {
        case 1: "One thing. You've got this."
        case 2: "Two things. You've got this."
        case 3: "Three things. You've got this."
        default: "A short list. You've got this."
        }
    }

    var footnote: String { "Everything else is okay." }

    var deferredLabel: String? {
        guard !deferred.isEmpty else { return nil }
        return deferred.count == 1
            ? "1 more for later today"
            : "\(deferred.count) more for later today"
    }
}

/// Stale is its own screen, with its own words, because "no action needed" and
/// "no fresh data since Tuesday" must never look alike.
struct StaleSummary: Sendable, Equatable {
    let since: Date
    let reason: StaleReason
    let now: Date
    /// Actions Planty already knew about. Still shown; just never as calm.
    let pending: [DigestEntry]

    var headline: String { "Planty needs a fresh look." }

    var body: String {
        "The last complete check was \(RelativeAge.phrase(since: since, now: now)). \(reason.sentence)"
    }

    var pendingLabel: String? {
        guard !pending.isEmpty else { return nil }
        return pending.count == 1
            ? "1 action from the last good check is still below."
            : "\(pending.count) actions from the last good check are still below."
    }
}

/// What the screen was handed, before it decides what to show.
struct TodayInputs: Sendable, Equatable {
    var isConfigured: Bool
    var isLoading: Bool
    var digest: Digest?
    var error: PlantyError?
    var knownPlantCount: Int?
    var now: Date
    var didJustFinish = false
}

extension TodayPresentation {
    static func make(
        _ inputs: TodayInputs,
        policy: FreshnessPolicy = .standard
    ) -> TodayPresentation {
        guard inputs.isConfigured else { return .unconfigured }

        if inputs.isLoading {
            if let digest = inputs.digest {
                return .loadingWarm(previous: digest, checkedAt: digest.date)
            }
            return .loadingCold
        }

        if let error = inputs.error {
            return .failed(error: error, cached: inputs.digest)
        }

        guard let digest = inputs.digest else { return .loadingCold }

        if digest.entries.isEmpty, inputs.knownPlantCount == 0 {
            return .emptySetup
        }

        let freshness = digest.freshness(
            now: inputs.now,
            knownPlantCount: inputs.knownPlantCount,
            policy: policy
        )
        if case .stale(let since, let reason) = freshness {
            return .stale(
                StaleSummary(
                    since: since,
                    reason: reason,
                    now: inputs.now,
                    pending: digest.sortedEntries
                )
            )
        }

        guard !digest.entries.isEmpty else {
            return .calm(
                CalmSummary(
                    checked: digest.checked,
                    updatedAt: digest.date,
                    justFinished: inputs.didJustFinish
                )
            )
        }

        let sorted = digest.sortedEntries
        return .actions(
            ActionSummary(
                featured: Array(sorted.prefix(ActionSummary.maxFeatured)),
                deferred: Array(sorted.dropFirst(ActionSummary.maxFeatured)),
                checked: digest.checked,
                updatedAt: digest.date
            )
        )
    }
}
