import Foundation
import Observation

/// Loads the daily digest and turns it into exactly one presentation. It keeps
/// the last good digest so a failed refresh never blanks the screen.
@Observable
@MainActor
final class TodayStore {
    private(set) var digest: Digest?
    private(set) var knownPlantCount: Int?
    private(set) var error: PlantyError?

    /// Kept apart from `error`, which is about the load. Tapping "Watered" and
    /// failing used to render "Today's check did not finish", which is a claim
    /// about the whole screen rather than about the tap.
    private(set) var actionError: PlantyError?

    /// The plant a note was just written against, for a brief confirmation.
    private(set) var noted: String?

    private(set) var isLoading = false
    private(set) var lastLoadedAt: Date?

    /// Cards the user postponed, and when they may come back.
    private(set) var postponedUntil: [UUID: Date] = [:]

    /// Verdicts settled on this device since the last load. Kept beside the
    /// digest rather than edited into it, so `checked` stays honest.
    private(set) var resolvedIDs: Set<UUID> = []

    var isConfigured: Bool
    private var api: any PlantyAPI
    private let policy: FreshnessPolicy
    private let clock: @Sendable () -> Date

    init(
        api: any PlantyAPI,
        isConfigured: Bool,
        policy: FreshnessPolicy = .standard,
        clock: @escaping @Sendable () -> Date = { Date() }
    ) {
        self.api = api
        self.isConfigured = isConfigured
        self.policy = policy
        self.clock = clock
    }

    func replace(api: any PlantyAPI, isConfigured: Bool) {
        self.api = api
        self.isConfigured = isConfigured
        digest = nil
        knownPlantCount = nil
        error = nil
        lastLoadedAt = nil
        resolvedIDs = []
        postponedUntil = [:]
    }

    var presentation: TodayPresentation {
        TodayPresentation.make(
            TodayInputs(
                isConfigured: isConfigured,
                isLoading: isLoading,
                digest: visibleDigest,
                error: error,
                knownPlantCount: knownPlantCount,
                now: clock(),
                didJustFinish: !resolvedIDs.isEmpty
            ),
            policy: policy
        )
    }

    /// Postponed cards drop out until their interval passes. They are never
    /// marked done: "Not now" and "I handled it" are different claims.
    private var visibleDigest: Digest? {
        guard let digest else { return nil }
        let now = clock()
        var hidden = resolvedIDs
        hidden.formUnion(postponedUntil.filter { $0.value > now }.keys)
        guard !hidden.isEmpty else { return digest }
        return digest.hiding(verdictIDs: hidden)
    }

    func load() async {
        guard isConfigured else { return }
        isLoading = true
        error = nil
        defer { isLoading = false }

        do {
            async let digestTask = api.today()
            async let plantsTask = api.plants(filter: .live)
            let (loaded, plants) = try await (digestTask, plantsTask)
            digest = loaded
            knownPlantCount = plants.count
            lastLoadedAt = clock()
        } catch {
            // A pull to refresh whose gesture ended is not a failed check, and
            // showing it as one is worse than showing nothing.
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
    }

    func acknowledge(_ entry: DigestEntry) async {
        do {
            try await api.acknowledge(verdictID: entry.verdict.id)
            resolvedIDs.insert(entry.verdict.id)
        } catch {
            actionError = PlantyError.from(error)
        }
    }

    /// Records what happened and drops the card, the only route by which an
    /// action may disappear as complete. The acknowledgement stops the service
    /// escalating, so its failure is reported rather than swallowed.
    func complete(_ entry: DigestEntry, kind: ObservationKind, note: String = "") async {
        do {
            _ = try await api.addObservation(
                slug: entry.plant.slug,
                observation: NewObservation(kind: kind, body: note.isEmpty ? nil : note)
            )
        } catch {
            actionError = PlantyError.from(error)
            return
        }

        do {
            try await api.acknowledge(verdictID: entry.verdict.id)
            resolvedIDs.insert(entry.verdict.id)
        } catch {
            actionError = .stillAsking
        }
    }

    /// Writes something down without claiming the job is done. The card stays,
    /// because a note is not a watering.
    func addNote(_ entry: DigestEntry, text: String) async {
        guard !text.isEmpty else { return }
        do {
            _ = try await api.addObservation(
                slug: entry.plant.slug,
                observation: NewObservation(kind: .note, body: text)
            )
            noted = entry.plant.commonName
        } catch {
            actionError = PlantyError.from(error)
        }
    }

    /// A verdict settled elsewhere, by a capture that answered its card. Today
    /// has to be told, or the card it just satisfied reappears on this screen.
    func settled(_ verdictID: UUID) {
        resolvedIDs.insert(verdictID)
    }

    func clearActionError() { actionError = nil }
    func clearNoted() { noted = nil }

    func postpone(_ entry: DigestEntry, by interval: PostponeInterval) {
        postponedUntil[entry.verdict.id] = clock().addingTimeInterval(interval.seconds)
    }

    func clearError() { error = nil }
}

enum PostponeInterval: String, CaseIterable, Sendable, Identifiable {
    case anHour
    case laterToday

    var id: String { rawValue }

    var label: String {
        switch self {
        case .anHour: "Remind me in 1 hour"
        case .laterToday: "Later today"
        }
    }

    var seconds: TimeInterval {
        switch self {
        case .anHour: 60 * 60
        case .laterToday: 5 * 60 * 60
        }
    }
}
