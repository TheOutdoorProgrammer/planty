import SwiftUI

/// The five words the app is allowed to use about a plant. There is no score,
/// and "All good" never means healthy, only that nothing is recommended.
enum CareState: Sendable, Equatable, CaseIterable {
    case allGood
    case watch
    case needsCare
    case urgent
    case unknown

    var label: String {
        switch self {
        case .allGood: "All good"
        case .watch: "Watch"
        case .needsCare: "Needs care"
        case .urgent: "Urgent"
        case .unknown: "Unknown"
        }
    }

    /// Deliberately never says healthy: a photo and a probe cannot prove that.
    var sentence: String {
        switch self {
        case .allGood: "No action needed on current evidence."
        case .watch: "Worth comparing later. Nothing to do today."
        case .needsCare: "One thing to do now."
        case .urgent: "Waiting could cause harm."
        case .unknown: "Planty has no current evidence for this plant."
        }
    }

    var symbol: String {
        switch self {
        case .allGood: "checkmark.circle.fill"
        case .watch: "eye.fill"
        case .needsCare: "drop.fill"
        case .urgent: "exclamationmark.triangle.fill"
        case .unknown: "questionmark.circle.fill"
        }
    }

    /// Red is only ever evidence of active harm.
    var color: Color {
        switch self {
        case .allGood: PlantyColor.green
        case .watch: PlantyColor.yellow
        case .needsCare: PlantyColor.orange
        case .urgent: PlantyColor.red
        case .unknown: PlantyColor.secondaryText
        }
    }

    var isActionable: Bool { self == .needsCare || self == .urgent }
}

extension CareState {
    /// Stale evidence can take reassurance away but never an alarm: an urgent
    /// verdict nobody refreshed is still the most useful thing Planty knows.
    static func resolve(verdict: Verdict?, freshness: Freshness) -> CareState {
        guard let verdict else { return .unknown }

        let stated = from(action: verdict.action)
        if freshness.isFresh { return stated }
        return stated == .allGood ? .unknown : stated
    }

    static func from(action: VerdictAction) -> CareState {
        switch action {
        case .none: .allGood
        case .check: .watch
        case .water, .harvest: .needsCare
        case .urgent: .urgent
        case .unknown: .unknown
        }
    }
}

/// The digest only lists plants needing something, so a plant's absence from it
/// is meaningful, but only while the digest itself is fresh.
enum LibraryStatus {
    static func state(
        for plant: Plant,
        digest: Digest?,
        now: Date,
        knownPlantCount: Int?,
        policy: FreshnessPolicy = .standard
    ) -> CareState {
        guard let digest else { return .unknown }
        let freshness = digest.freshness(
            now: now,
            knownPlantCount: knownPlantCount,
            policy: policy
        )
        let entry = digest.entries.first { $0.plant.id == plant.id }
        return CareState.resolve(verdict: entry?.verdict, freshness: freshness)
            .fallingBackToAllGood(whenAbsent: entry == nil, freshness: freshness)
    }
}

private extension CareState {
    /// A plant the run checked and did not flag is All good, but only when the
    /// run itself is fresh. Otherwise it stays Unknown.
    func fallingBackToAllGood(whenAbsent absent: Bool, freshness: Freshness) -> CareState {
        guard absent else { return self }
        return freshness.isFresh ? .allGood : .unknown
    }
}

extension VerdictAction {
    /// The one physical instruction a card leads with, before any explanation.
    var instruction: String {
        switch self {
        case .water: "Give it a slow drink. Stop when water reaches the tray."
        case .check: "Take a look next time you walk past."
        case .harvest: "It is ready to pick."
        case .urgent: "Deal with this one before the others."
        case .none, .unknown: "Nothing to do."
        }
    }

    var shortLabel: String {
        switch self {
        case .water: "Needs water"
        case .check: "Worth a look"
        case .harvest: "Ready to harvest"
        case .urgent: "Needs you now"
        case .none: "All good"
        case .unknown: "Unclear"
        }
    }
}
