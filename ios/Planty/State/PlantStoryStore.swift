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
    private(set) var hasLoaded = false

    private let api: any PlantyAPI
    private let policy: FreshnessPolicy
    private let clock: @Sendable () -> Date

    init(
        api: any PlantyAPI,
        plant: Plant,
        policy: FreshnessPolicy = .standard,
        clock: @escaping @Sendable () -> Date = { Date() }
    ) {
        self.api = api
        self.plant = plant
        self.policy = policy
        self.clock = clock
    }

    var chapters: [StoryChapter] { StoryBuilder.chapters(from: timeline) }
    var series: [SensorSeries] { timeline.series }

    var verdict: Verdict? {
        detail?.verdict ?? timeline.verdicts.max { $0.forDate < $1.forDate }
    }

    /// A verdict older than the policy allows cannot make this plant look calm.
    var freshness: Freshness {
        guard let verdict else { return .stale(since: .distantPast, reason: .checkedNothing) }
        let age = clock().timeIntervalSince(verdict.forDate)
        return age > policy.maxAge ? .stale(since: verdict.forDate, reason: .tooOld) : .fresh
    }

    var careState: CareState {
        CareState.resolve(verdict: verdict, freshness: freshness)
    }

    /// "Last compared today at 8:04 AM", or an honest admission there is none.
    var lastComparedLine: String {
        guard let verdict else { return "Planty has not compared anything yet." }
        return "Last compared \(RelativeAge.dayAndTime(verdict.forDate, now: clock()))"
    }

    var hasPhotos: Bool { !timeline.photos.isEmpty }
    var hasStory: Bool { !timeline.isEmpty }

    /// Answers the cold warning from the phone it arrived on. The local state
    /// moves straight away, so the button does not keep offering what was just
    /// done while the next load is still in flight.
    func setSheltered(_ indoors: Bool) async {
        do {
            _ = try await api.shelter(slugs: [plant.slug], indoors: indoors)
            plant.shelteredAt = indoors ? clock() : nil
            error = nil
        } catch {
            self.error = PlantyError.from(error)
        }
    }

    func load() async {
        isLoading = true
        error = nil
        defer { isLoading = false }

        do {
            async let detailTask = api.plant(slug: plant.slug)
            async let timelineTask = api.timeline(slug: plant.slug)
            let (loadedDetail, loadedTimeline) = try await (detailTask, timelineTask)
            detail = loadedDetail
            plant = loadedDetail.plant
            timeline = loadedTimeline.merging(loadedDetail)
            hasLoaded = true
        } catch {
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
    }

    /// Failures come back to the sheet that asked instead of landing in
    /// `error`, which the screen reads as "the story did not load".
    func record(_ kind: ObservationKind, note: String? = nil) async -> PlantyError? {
        do {
            let created = try await api.addObservation(
                slug: plant.slug,
                observation: NewObservation(kind: kind, body: note)
            )
            timeline.observations.insert(created, at: 0)
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
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    /// Irreversible from the app, which is why the screen confirms first. The
    /// local copy moves too so the page reads dead without another fetch.
    func markDead() async -> PlantyError? {
        do {
            try await api.archivePlant(slug: plant.slug, status: .dead)
            plant.status = .dead
            plant.archivedAt = clock()
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
