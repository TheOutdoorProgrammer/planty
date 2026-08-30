import Foundation
import Testing

@testable import Planty

/// Suspends until cancelled, which is what a request in flight when the
/// pull-to-refresh gesture ends actually does.
private final class HangingAPI: PlantyAPI, @unchecked Sendable {
    func today() async throws -> Digest {
        try await Task.sleep(for: .seconds(60))
        return .fixture()
    }

    func plants(filter: PlantFilter) async throws -> [Plant] {
        try await Task.sleep(for: .seconds(60))
        return []
    }

    func plant(slug: String) async throws -> PlantDetail {
        try await Task.sleep(for: .seconds(60))
        return PlantDetail(plant: .fixture())
    }

    func timeline(slug: String) async throws -> PlantTimeline {
        try await Task.sleep(for: .seconds(60))
        return PlantTimeline()
    }

    func createPlant(_ draft: NewPlant) async throws -> Plant { throw PlantyError.notFound }
    func updatePlant(slug: String, patch: PlantPatch) async throws -> Plant { throw PlantyError.notFound }
    func archivePlant(slug: String, status: PlantStatus) async throws {}
    func addObservation(slug: String, observation: NewObservation) async throws -> PlantObservation {
        throw PlantyError.notFound
    }
    func acknowledge(verdictID: UUID) async throws {}
    func sensors() async throws -> [SensorSeries] { [] }
    func shelter(slugs: [String], indoors: Bool) async throws -> Int { 0 }
    func calibrate(sensorID: UUID, to calibration: SensorCalibration) async throws -> SensorLink {
        throw PlantyError.notFound
    }
    func identify(jpeg: Data, metadata: CaptureMetadata) async throws -> [IdentificationCandidate] { [] }
    func logHarvest(_ harvest: NewHarvest, on slug: String) async throws -> Harvest {
        throw PlantyError.notFound
    }
    func harvests(slug: String?) async throws -> [Harvest] {
        try await Task.sleep(for: .seconds(60))
        return []
    }
    func uploadPhoto(slug: String, jpeg: Data, caption: String?, takenAt: Date) async throws -> Photo {
        throw PlantyError.notFound
    }
    func ask(slug: String, question: PlantQuestion) async throws -> PlantAnswer {
        try await Task.sleep(for: .seconds(60))
        throw PlantyError.notFound
    }
    func ask(_ question: ScratchQuestion) async throws -> PlantAnswer {
        try await Task.sleep(for: .seconds(60))
        throw PlantyError.notFound
    }
    func reminders(slug: String) async throws -> [Reminder] {
        try await Task.sleep(for: .seconds(60))
        return []
    }
    func notes(slug: String) async throws -> [PlantNote] {
        try await Task.sleep(for: .seconds(60))
        return []
    }

    func answerQuestion(id: UUID, answer: String) async throws {
        try await Task.sleep(for: .seconds(60))
    }
    func postmortems() async throws -> [Postmortem] {
        try await Task.sleep(for: .seconds(60))
        return []
    }
    func questions(status: QuestionStatus) async throws -> [OpenQuestion] {
        try await Task.sleep(for: .seconds(60))
        return []
    }
    func createQuestion(_ draft: NewOpenQuestion) async throws -> OpenQuestion {
        throw PlantyError.notFound
    }
    func planAway(_ draft: NewAwayPeriod) async throws -> AwayPeriod {
        throw PlantyError.notFound
    }
    func coldWatch(forecastLowF: Double) async throws -> ColdWatch {
        throw PlantyError.notFound
    }
    func householdNotes() async throws -> [PlantNote] {
        try await Task.sleep(for: .seconds(60))
        return []
    }
    func addHouseholdNote(draft: NoteDraft) async throws -> PlantNote {
        try await Task.sleep(for: .seconds(60))
        throw PlantyError.offline
    }
    func addNote(slug: String, draft: NoteDraft) async throws -> PlantNote {
        try await Task.sleep(for: .seconds(60))
        throw PlantyError.offline
    }
    func updateNote(id: UUID, draft: NoteDraft) async throws -> PlantNote {
        try await Task.sleep(for: .seconds(60))
        throw PlantyError.offline
    }
    func deleteNote(id: UUID) async throws {
        try await Task.sleep(for: .seconds(60))
    }
    func setReminder(slug: String, reminder: NewReminder) async throws -> Reminder {
        throw PlantyError.notFound
    }
    func deleteReminder(slug: String, kind: ObservationKind) async throws {}
    func createPlantFromPhoto(_ ask: PlantFromPhoto) async throws -> PlantFromPhotoResult {
        try await Task.sleep(for: .seconds(60))
        throw PlantyError.notFound
    }
    func linkSensor(_ link: NewSensorLink) async throws -> SensorLink { throw PlantyError.notFound }
    func postmortem(slug: String) async throws -> Postmortem { throw PlantyError.notFound }
    func health() async throws {}
}

/// Opening the app uses `.task`, which lives as long as the view. Pulling to
/// refresh uses `.refreshable`, whose task is cancelled when the gesture ends.
/// Same `load()`, and only one of them was showing an error.
@Suite("Refresh cancellation")
struct RefreshCancellationTests {
    @Test("A cancelled refresh is not a failure")
    @MainActor
    func cancellationIsNotAnError() async {
        let store = TodayStore(api: HangingAPI(), isConfigured: true)

        let refresh = Task { await store.load() }
        // Let load() get as far as its first await before pulling the rug.
        try? await Task.sleep(for: .milliseconds(50))
        refresh.cancel()
        _ = await refresh.value

        #expect(store.error == nil, "a cancelled refresh rendered as \"Today's check did not finish\"")
    }

    /// Three screens pull to refresh, so fixing only the one that was reported
    /// leaves the same bug on the other two.
    @Test("Every screen that pulls to refresh survives cancellation")
    @MainActor
    func everyRefreshPathSurvives() async {
        let plants = PlantsStore(api: HangingAPI(), isConfigured: true)
        let story = PlantStoryStore(api: HangingAPI(), plant: .fixture())

        let first = Task { await plants.load() }
        let second = Task { await story.load() }
        try? await Task.sleep(for: .milliseconds(50))
        first.cancel()
        second.cancel()
        _ = await first.value
        _ = await second.value

        #expect(plants.error == nil)
        #expect(story.error == nil)
    }

    /// The tests above cancel a `Task.sleep`, so the store sees a raw
    /// `CancellationError`. The real app never does: the client converts every
    /// error at its boundary first, which is exactly where this broke.
    @Test("A cancellation that came through the client is still a cancellation")
    @MainActor
    func cancellationSurvivesTheClientBoundary() async {
        let asClientReportsIt = PlantyError.from(URLError(.cancelled))

        #expect(asClientReportsIt == .cancelled, "the client flattened it to \(asClientReportsIt)")
        #expect(PlantyError.isCancellation(asClientReportsIt))
        #expect(PlantyError.isCancellation(URLError(.cancelled)))
        #expect(PlantyError.isCancellation(CancellationError()))

        let api = FakeAPI()
        api.failure = asClientReportsIt
        let store = TodayStore(api: api, isConfigured: true)
        await store.load()

        #expect(store.error == nil, "a cancelled refresh rendered as a failed check")
    }

    /// A real failure still has to reach the screen, or this fix has swapped a
    /// noisy bug for a silent one.
    @Test("A real failure is still reported")
    @MainActor
    func genuineFailuresStillSurface() async {
        let api = FakeAPI()
        api.failure = .offline
        let store = TodayStore(api: api, isConfigured: true)
        await store.load()

        #expect(store.error == .offline)
    }

    /// The spinner has to stop even when the work it was waiting on was thrown
    /// away, or the view is left claiming it is still loading.
    @Test("A cancelled refresh stops loading")
    @MainActor
    func cancellationClearsLoading() async {
        let store = TodayStore(api: HangingAPI(), isConfigured: true)

        let refresh = Task { await store.load() }
        try? await Task.sleep(for: .milliseconds(50))
        refresh.cancel()
        _ = await refresh.value

        #expect(!store.isLoading)
    }
}
