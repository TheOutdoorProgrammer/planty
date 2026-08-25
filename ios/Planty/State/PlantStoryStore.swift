import Foundation
import Observation

/// One plant's page: the current verdict, the story, and the sensor evidence
/// that stays folded away until somebody asks why.
@Observable
@MainActor
final class PlantStoryStore {
    private(set) var plant: Plant
    private(set) var detail: PlantDetail?
    private(set) var timeline = PlantTimeline()
    private(set) var error: PlantyError?
    private(set) var isLoading = false
    private(set) var isLoadingEarlier = false
    private(set) var isAssessing = false
    private(set) var hasLoaded = false

    private let api: any PlantyAPI
    private let policy: FreshnessPolicy
    private let clock: @Sendable () -> Date
    private let isSessionCurrent: @MainActor () -> Bool
    private var loadGeneration = 0
    private var observationCursor: String?
    private var photoCursor: String?

    init(
        api: any PlantyAPI,
        plant: Plant,
        policy: FreshnessPolicy = .standard,
        clock: @escaping @Sendable () -> Date = { Date() },
        isSessionCurrent: @escaping @MainActor () -> Bool = { true }
    ) {
        self.api = api
        self.plant = plant
        self.policy = policy
        self.clock = clock
        self.isSessionCurrent = isSessionCurrent
    }

    var chapters: [StoryChapter] { StoryBuilder.chapters(from: timeline) }
    var series: [SensorSeries] { timeline.series }

    /// The envelope wins over the plant inside it, and either will do: reading
    /// only one would show nothing if the service moved it.
    var toxicity: Toxicity? { detail?.toxicity ?? plant.toxicity }

    var verdict: Verdict? {
        detail?.verdict ?? timeline.verdicts.max { $0.forDate < $1.forDate }
    }

    /// A verdict older than the policy allows cannot make this plant look calm.
    var freshness: Freshness {
        guard let verdict else { return .stale(since: .distantPast, reason: .checkedNothing) }
        let age = clock().timeIntervalSince(verdict.createdAt)
        return age > policy.maxAge ? .stale(since: verdict.createdAt, reason: .tooOld) : .fresh
    }

    var careState: CareState {
        CareState.resolve(verdict: verdict, freshness: freshness)
    }

    /// "Last compared today at 8:04 AM", or an honest admission there is none.
    var lastComparedLine: String {
        guard let verdict else { return "Planty has not compared anything yet." }
        return "Last compared \(RelativeAge.dayAndTime(verdict.createdAt, now: clock()))"
    }

    var hasPhotos: Bool { !timeline.photos.isEmpty }
    var hasStory: Bool { !timeline.isEmpty }
    var hasEarlierHistory: Bool { observationCursor != nil || photoCursor != nil }

    /// Answers the cold warning from the phone it arrived on. The local state
    /// moves straight away, so the button does not keep offering what was just
    /// done while the next load is still in flight.
    @discardableResult
    func setSheltered(_ indoors: Bool) async -> PlantyError? {
        do {
            _ = try await api.shelter(slugs: [plant.slug], indoors: indoors)
            plant.shelteredAt = indoors ? clock() : nil
            error = nil
            return nil
        } catch {
            let failure = PlantyError.from(error)
            self.error = failure
            return failure
        }
    }

    func load() async {
        loadGeneration += 1
        let generation = loadGeneration
        let client = api
        let slug = plant.slug
        isLoading = true
        error = nil
        defer {
            if generation == loadGeneration { isLoading = false }
        }

        do {
            async let detailTask = client.plant(slug: slug)
            async let timelineTask = client.timeline(slug: slug)
            let (loadedDetail, loadedTimeline) = try await (detailTask, timelineTask)
            guard generation == loadGeneration, isSessionCurrent() else { return }
            detail = loadedDetail
            plant = loadedDetail.plant
            timeline = loadedTimeline.merging(loadedDetail)
            observationCursor = loadedDetail.observationsNextCursor
            photoCursor = loadedTimeline.nextCursor
            hasLoaded = true

            // Publish the first page immediately, then continue backward until
            // the server says history is exhausted. The story therefore never
            // silently stops at 20 observations or 24 photos.
            await loadRemainingHistory(generation: generation)
        } catch {
            guard generation == loadGeneration, isSessionCurrent() else { return }
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
    }

    private func loadRemainingHistory(generation: Int) async {
        while hasEarlierHistory, generation == loadGeneration, isSessionCurrent() {
            let beforeObservations = observationCursor
            let beforePhotos = photoCursor
            await loadEarlier()
            if beforeObservations == observationCursor, beforePhotos == photoCursor {
                break
            }
        }
    }

    /// Loads the next older page for each history stream that still has one.
    /// Pages publish together so a failure never leaves photos and care events
    /// at silently different depths.
    func loadEarlier() async {
        guard hasEarlierHistory, !isLoadingEarlier, isSessionCurrent() else { return }
        let generation = loadGeneration
        let client = api
        let slug = plant.slug
        let startingObservationCursor = observationCursor
        let startingPhotoCursor = photoCursor
        isLoadingEarlier = true
        error = nil
        defer { isLoadingEarlier = false }

        do {
            var olderObservations: [PlantObservation] = []
            var olderPhotos: [Photo] = []
            var nextObservationCursor = startingObservationCursor
            var nextPhotoCursor = startingPhotoCursor

            if let cursor = startingObservationCursor {
                let page = try await client.observationsPage(slug: slug, cursor: cursor)
                olderObservations = page.observations
                nextObservationCursor = page.nextCursor?.isEmpty == true ? nil : page.nextCursor
            }
            guard generation == loadGeneration, isSessionCurrent() else { return }

            if let cursor = startingPhotoCursor {
                let page = try await client.timelinePage(slug: slug, cursor: cursor)
                olderPhotos = page.photos
                nextPhotoCursor = page.nextCursor
            }
            guard generation == loadGeneration, isSessionCurrent() else { return }

            timeline = timeline.appending(observations: olderObservations, photos: olderPhotos)
            observationCursor = nextObservationCursor
            photoCursor = nextPhotoCursor
            timeline.nextCursor = nextPhotoCursor
        } catch {
            guard generation == loadGeneration, isSessionCurrent() else { return }
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
    }

    /// Failures come back to the sheet that asked instead of landing in
    /// `error`, which the screen reads as "the story did not load".
    func record(_ kind: ObservationKind, note: String? = nil) async -> PlantyError? {
        switch await recordEntry(kind, note: note) {
        case .success: nil
        case .failure(let error): error
        }
    }

    func recordEntry(
        _ kind: ObservationKind,
        note: String? = nil
    ) async -> Result<PlantObservation, PlantyError> {
        do {
            let created = try await api.addObservation(
                slug: plant.slug,
                observation: NewObservation(kind: kind, body: note)
            )
            timeline.observations.insert(created, at: 0)
            return .success(created)
        } catch {
            return .failure(PlantyError.from(error))
        }
    }

    func assessNow() async -> PlantyError? {
        guard !isAssessing else { return nil }
        isAssessing = true
        defer { isAssessing = false }

        do {
            let fresh = try await api.assess(slug: plant.slug)
            detail?.verdict = fresh
            timeline.verdicts.removeAll { $0.id == fresh.id }
            timeline.verdicts.insert(fresh, at: 0)
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    /// Sends only what changed; an empty patch never touches the network.
    func saveEdits(_ patch: PlantPatch) async -> PlantyError? {
        guard !patch.isEmpty else { return nil }
        do {
            let updated = try await api.updatePlant(slug: plant.slug, patch: patch)
            plant = updated.keepingPhoto(from: plant)
            detail?.toxicity = plant.toxicity
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    func archive(as status: PlantStatus) async -> PlantyError? {
        do {
            try await api.archivePlant(slug: plant.slug, status: status)
            plant.status = status
            plant.archivedAt = clock()
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    func markDead() async -> PlantyError? {
        await archive(as: .dead)
    }

    func restore() async -> PlantyError? {
        do {
            plant = try await api.restorePlant(slug: plant.slug)
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    func deletePhoto(_ photo: Photo) async -> PlantyError? {
        do {
            try await api.deletePhoto(id: photo.id)
            timeline.photos.removeAll { $0.id == photo.id }
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    func logHarvest(quantity: Double, unit: String, notes: String?) async -> PlantyError? {
        do {
            _ = try await api.logHarvest(
                NewHarvest(occurredAt: clock(), quantity: quantity, unit: unit, notes: notes),
                on: plant.slug
            )
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    private(set) var postmortem: Postmortem?
    private(set) var postmortemError: PlantyError?
    private(set) var isAskingPostmortem = false

    /// The service reads the whole story before answering, so this can take a
    /// minute. The flag is what keeps the screen honest about that.
    func askPostmortem() async {
        guard !isAskingPostmortem else { return }
        isAskingPostmortem = true
        postmortemError = nil
        defer { isAskingPostmortem = false }
        do {
            postmortem = try await api.postmortem(slug: plant.slug)
        } catch {
            guard !PlantyError.isCancellation(error) else { return }
            postmortemError = PlantyError.from(error)
        }
    }

    func clearError() { error = nil }
}
