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

    /// Reminder identity includes its scheduled slot. Completing an 8 AM mist
    /// must not hide the same reminder when its 8 PM occurrence becomes due.
    private(set) var resolvedReminderOccurrenceIDs: Set<String> = []
    private(set) var completingReminderOccurrenceIDs: Set<String> = []

    var isConfigured: Bool
    private var api: any PlantyAPI
    private let policy: FreshnessPolicy
    private let clock: @Sendable () -> Date
    private var loadGeneration = 0
    private var completionAttempts: [UUID: CompletionAttempt] = [:]
    private var reminderCompletionAttempts: [String: UUID] = [:]

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
        loadGeneration += 1
        self.api = api
        self.isConfigured = isConfigured
        digest = nil
        knownPlantCount = nil
        error = nil
        actionError = nil
        noted = nil
        lastLoadedAt = nil
        isLoading = false
        resolvedIDs = []
        resolvedReminderOccurrenceIDs = []
        completingReminderOccurrenceIDs = []
        postponedUntil = [:]
        completionAttempts = [:]
        reminderCompletionAttempts = [:]
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
                didJustFinish: !resolvedIDs.isEmpty || !resolvedReminderOccurrenceIDs.isEmpty
            ),
            policy: policy
        )
    }

    /// Postponed verdict cards drop out until their interval passes. They are
    /// never marked done: "Later" and "I handled it" are different claims.
    private var visibleDigest: Digest? {
        guard let digest else { return nil }
        let now = clock()
        var hiddenVerdicts = resolvedIDs
        hiddenVerdicts.formUnion(postponedUntil.filter { $0.value > now }.keys)
        guard !hiddenVerdicts.isEmpty || !resolvedReminderOccurrenceIDs.isEmpty else {
            return digest
        }
        return digest.hiding(
            verdictIDs: hiddenVerdicts,
            reminderOccurrenceIDs: resolvedReminderOccurrenceIDs
        )
    }

    func load() async {
        guard isConfigured else { return }
        loadGeneration += 1
        let generation = loadGeneration
        let client = api
        isLoading = true
        error = nil
        defer {
            if generation == loadGeneration { isLoading = false }
        }

        do {
            async let digestTask = client.today()
            async let plantsTask = client.plants(filter: .live)
            let (loaded, plants) = try await (digestTask, plantsTask)
            guard generation == loadGeneration else { return }
            digest = loaded.keepingPhotos(from: plants)
            knownPlantCount = plants.count
            lastLoadedAt = clock()
        } catch {
            guard generation == loadGeneration else { return }
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

    /// The service records care and acknowledges the verdict in one idempotent
    /// transaction. Keep the same key after a transport failure so retrying can
    /// never duplicate the observation if the first response was merely lost.
    @discardableResult
    func complete(
        _ entry: DigestEntry,
        kind: ObservationKind,
        note: String = ""
    ) async -> PlantyError? {
        actionError = nil
        let identity = CompletionIdentity(kind: kind, note: note)
        let attempt: CompletionAttempt
        if let pending = completionAttempts[entry.verdict.id], pending.identity == identity {
            attempt = pending
        } else {
            attempt = CompletionAttempt(id: UUID(), identity: identity)
            completionAttempts[entry.verdict.id] = attempt
        }

        do {
            _ = try await api.completeVerdict(
                slug: entry.plant.slug,
                verdictID: entry.verdict.id,
                kind: kind,
                note: note,
                idempotencyKey: attempt.id
            )
            completionAttempts.removeValue(forKey: entry.verdict.id)
            resolvedIDs.insert(entry.verdict.id)
            return nil
        } catch {
            let failure = PlantyError.from(error)
            actionError = failure
            return failure
        }
    }

    /// Completing a scheduled reminder records exactly the kind it was created
    /// for. Production sends one idempotent occurrence to the server; the plain
    /// observation fallback exists only for lightweight test doubles.
    @discardableResult
    func complete(_ occurrence: DueReminder) async -> PlantyError? {
        let identity = occurrence.occurrenceID
        guard !completingReminderOccurrenceIDs.contains(identity) else { return nil }

        actionError = nil
        completingReminderOccurrenceIDs.insert(identity)
        defer { completingReminderOccurrenceIDs.remove(identity) }

        let idempotencyKey = reminderCompletionAttempts[identity] ?? UUID()
        reminderCompletionAttempts[identity] = idempotencyKey

        do {
            if let completing = api as? any ReminderCompleting {
                _ = try await completing.completeReminder(
                    reminderID: occurrence.reminder.id,
                    dueAt: occurrence.dueAt,
                    idempotencyKey: idempotencyKey
                )
            } else {
                _ = try await api.addObservation(
                    slug: occurrence.plant.slug,
                    observation: NewObservation(
                        kind: occurrence.reminder.kind,
                        body: occurrence.reminder.note
                    )
                )
            }
            reminderCompletionAttempts.removeValue(forKey: identity)
            resolvedReminderOccurrenceIDs.insert(identity)
            return nil
        } catch {
            let failure = PlantyError.from(error)
            actionError = failure
            return failure
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

private struct CompletionIdentity: Equatable {
    let kind: ObservationKind
    let note: String
}

private struct CompletionAttempt {
    let id: UUID
    let identity: CompletionIdentity
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
