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

    /// Kept apart from `error`, which is about the load. Tapping a care action
    /// and failing must not turn into a claim that the whole daily check failed.
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
        actionError = nil
        noted = nil
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
    /// marked done: "Later" and "I handled it" are different claims.
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
            digest = loaded.keepingPhotos(from: plants)
            knownPlantCount = plants.count
            lastLoadedAt = clock()
        } catch {
            // A pull to refresh whose gesture ended is not a failed check, and
            // showing it as one is worse than showing nothing.
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
    }

    /// Explicit acknowledgement is intentionally different from recording care:
    /// it says "I saw this" and silences the current verdict without inventing
    /// a watering, move or symptom that did not happen.
    @discardableResult
    func acknowledge(_ entry: DigestEntry) async -> PlantyError? {
        actionError = nil
        do {
            try await api.acknowledge(verdictID: entry.verdict.id)
            resolvedIDs.insert(entry.verdict.id)
            return nil
        } catch {
            let failure = PlantyError.from(error)
            actionError = failure
            return failure
        }
    }

    /// Waiting on a person rather than a plant, and answered from here so the
    /// queue has an outlet instead of filling up unread.
    var openQuestions: [OpenQuestion] { digest?.openQuestions ?? [] }

    func answer(_ question: OpenQuestion, with words: String) async -> PlantyError? {
        let said = words.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !said.isEmpty else { return nil }
        do {
            try await api.answerQuestion(id: question.id, answer: said)
            await load()
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    /// Records what happened and then acknowledges the verdict. A failure in
    /// either half leaves the card visible and is returned to the action screen.
    @discardableResult
    func complete(
        _ entry: DigestEntry,
        kind: ObservationKind,
        note: String = ""
    ) async -> PlantyError? {
        actionError = nil
        do {
            _ = try await api.addObservation(
                slug: entry.plant.slug,
                observation: NewObservation(kind: kind, body: note.isEmpty ? nil : note)
            )
        } catch {
            let failure = PlantyError.from(error)
            actionError = failure
            return failure
        }

        do {
            try await api.acknowledge(verdictID: entry.verdict.id)
            resolvedIDs.insert(entry.verdict.id)
            return nil
        } catch {
            actionError = .stillAsking
            return .stillAsking
        }
    }

    /// A photograph is optional evidence. Saving one never silently marks a
    /// verdict handled; the user still chooses acknowledgement or a care action.
    @discardableResult
    func addPhoto(_ entry: DigestEntry, jpeg: Data) async -> PlantyError? {
        actionError = nil
        do {
            _ = try await api.uploadPhoto(
                slug: entry.plant.slug,
                jpeg: jpeg,
                caption: nil,
                takenAt: clock()
            )
            return nil
        } catch {
            let failure = PlantyError.from(error)
            actionError = failure
            return failure
        }
    }

    /// Writes something down without claiming the job is done. The card stays,
    /// because a note is not a watering or an acknowledgement.
    func addNote(_ entry: DigestEntry, text: String) async {
        guard !text.isEmpty else { return }
        actionError = nil
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
