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
            self.error = PlantyError.from(error)
        }
    }

    func record(_ kind: ObservationKind, note: String? = nil) async {
        do {
            let created = try await api.addObservation(
                slug: plant.slug,
                observation: NewObservation(kind: kind, body: note)
            )
            timeline.observations.insert(created, at: 0)
        } catch {
            self.error = PlantyError.from(error)
        }
    }

    func clearError() { error = nil }
}
