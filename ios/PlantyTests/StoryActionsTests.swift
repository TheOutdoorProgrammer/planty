import Foundation
import Testing

@testable import Planty

/// Answers with what the test set and records every write verbatim. FakeAPI
/// predates the write paths and keeps none of what they send.
final class RecordingAPI: PlantyAPI, @unchecked Sendable {
    private let lock = NSLock()
    private var _plantReturned: Plant = .fixture()
    private var _postmortemReturned: Postmortem = .fixture()
    private var _failure: PlantyError?
    private var _patches: [(String, PlantPatch)] = []
    private var _archived: [(String, PlantStatus)] = []
    private var _observations: [(String, NewObservation)] = []
    private var _harvests: [(String, NewHarvest)] = []
    private var _postmortemAsks: [String] = []

    var plantReturned: Plant {
        get { lock.withLock { _plantReturned } }
        set { lock.withLock { _plantReturned = newValue } }
    }

    var postmortemReturned: Postmortem {
        get { lock.withLock { _postmortemReturned } }
        set { lock.withLock { _postmortemReturned = newValue } }
    }

    var failure: PlantyError? {
        get { lock.withLock { _failure } }
        set { lock.withLock { _failure = newValue } }
    }

    var patches: [(String, PlantPatch)] { lock.withLock { _patches } }
    var archived: [(String, PlantStatus)] { lock.withLock { _archived } }
    var observations: [(String, NewObservation)] { lock.withLock { _observations } }
    var harvests: [(String, NewHarvest)] { lock.withLock { _harvests } }
    var postmortemAsks: [String] { lock.withLock { _postmortemAsks } }

    private func check() throws {
        if let failure { throw failure }
    }

    func plants(filter: PlantFilter) async throws -> [Plant] {
        try check()
        return [plantReturned]
    }

    func plant(slug: String) async throws -> PlantDetail {
        try check()
        return PlantDetail(plant: plantReturned)
    }

    func createPlant(_ draft: NewPlant) async throws -> Plant {
        try check()
        return .fixture(commonName: draft.commonName)
    }

    func updatePlant(slug: String, patch: PlantPatch) async throws -> Plant {
        try check()
        lock.withLock { _patches.append((slug, patch)) }
        return plantReturned
    }

    func archivePlant(slug: String, status: PlantStatus) async throws {
        try check()
        lock.withLock { _archived.append((slug, status)) }
    }

    func addObservation(slug: String, observation: NewObservation) async throws -> PlantObservation {
        try check()
        lock.withLock { _observations.append((slug, observation)) }
        return PlantObservation(
            id: UUID(),
            plantID: UUID(),
            kind: observation.kind,
            body: observation.body,
            occurredAt: observation.occurredAt,
            source: observation.source,
            createdAt: observation.occurredAt
        )
    }

    func timeline(slug: String) async throws -> PlantTimeline {
        try check()
        return PlantTimeline()
    }

    func logHarvest(_ harvest: NewHarvest, on slug: String) async throws -> Harvest {
        try check()
        lock.withLock { _harvests.append((slug, harvest)) }
        return Harvest(
            id: UUID(),
            plantID: UUID(),
            occurredAt: harvest.occurredAt,
            quantity: harvest.quantity,
            unit: harvest.unit,
            notes: harvest.notes,
            createdAt: harvest.occurredAt
        )
    }

    func postmortem(slug: String) async throws -> Postmortem {
        try check()
        lock.withLock { _postmortemAsks.append(slug) }
        return postmortemReturned
    }

    // The rest of the contract, unused by these stores' write paths.
    func today() async throws -> Digest { throw PlantyError.notFound }

    func uploadPhoto(slug: String, jpeg: Data, caption: String?, takenAt: Date) async throws -> Photo {
        throw PlantyError.notFound
    }

    func acknowledge(verdictID: UUID) async throws { throw PlantyError.notFound }
    func sensors() async throws -> [SensorLink] { throw PlantyError.notFound }

    func calibrate(sensorID: UUID, to calibration: SensorCalibration) async throws -> SensorLink {
        throw PlantyError.notFound
    }

    func shelter(slugs: [String], indoors: Bool) async throws -> Int { throw PlantyError.notFound }

    func identify(jpeg: Data, metadata: CaptureMetadata) async throws -> [IdentificationCandidate] {
        throw PlantyError.notFound
    }

    func ask(slug: String, question: PlantQuestion) async throws -> PlantAnswer {
        throw PlantyError.notFound
    }

    func ask(_ question: ScratchQuestion) async throws -> PlantAnswer {
        throw PlantyError.notFound
    }

    func createPlantFromPhoto(_ request: PlantFromPhoto) async throws -> PlantFromPhotoResult {
        throw PlantyError.notFound
    }

    func linkSensor(_ link: NewSensorLink) async throws -> SensorLink { throw PlantyError.notFound }
    func reminders(slug: String) async throws -> [Reminder] { throw PlantyError.notFound }

    func notes(slug: String) async throws -> [PlantNote] { [] }

    func answerQuestion(id: UUID, answer: String) async throws { throw PlantyError.notFound }
    func householdNotes() async throws -> [PlantNote] { [] }
    func addHouseholdNote(draft: NoteDraft) async throws -> PlantNote {
        throw PlantyError.notFound
    }
    func addNote(slug: String, draft: NoteDraft) async throws -> PlantNote {
        throw PlantyError.notFound
    }
    func updateNote(id: UUID, draft: NoteDraft) async throws -> PlantNote {
        throw PlantyError.notFound
    }
    func deleteNote(id: UUID) async throws { throw PlantyError.notFound }

    func setReminder(slug: String, reminder: NewReminder) async throws -> Reminder {
        throw PlantyError.notFound
    }

    func deleteReminder(slug: String, kind: ObservationKind) async throws {
        throw PlantyError.notFound
    }

    func harvests(slug: String?) async throws -> [Harvest] { [] }
    func postmortems() async throws -> [Postmortem] { [] }
    func questions(status: QuestionStatus) async throws -> [OpenQuestion] { [] }
    func createQuestion(_ draft: NewOpenQuestion) async throws -> OpenQuestion {
        throw PlantyError.notFound
    }
    func planAway(_ draft: NewAwayPeriod) async throws -> AwayPeriod {
        throw PlantyError.notFound
    }
    func coldWatch(forecastLowF: Double) async throws -> ColdWatch {
        throw PlantyError.notFound
    }

    func health() async throws { throw PlantyError.notFound }
}

extension Postmortem {
    static func fixture(lesson: String? = "Water less in winter.") -> Postmortem {
        Postmortem(
            id: UUID(),
            plantID: UUID(),
            likelyCause: "Overwatering",
            narrative: "The soil never dried out between waterings.",
            lesson: lesson,
            createdAt: .reference
        )
    }
}

/// Editing, logging care, recording a death and asking what killed it, all
/// from the plant's own page.
@Suite("Acting on a plant from its page")
struct StoryActionsTests {
    @Test("An edit sends the patch verbatim and adopts the answer")
    @MainActor
    func saveEditsSendsThePatch() async {
        let api = RecordingAPI()
        api.plantReturned = .fixture(slug: "mona", commonName: "Monstera")
        let store = PlantStoryStore(api: api, plant: .fixture(slug: "mona"))

        var patch = PlantPatch()
        patch.commonName = "Monstera"
        patch.minTempF = 45
        let failure = await store.saveEdits(patch)

        #expect(failure == nil)
        #expect(api.patches.count == 1)
        #expect(api.patches.first?.0 == "mona")
        #expect(api.patches.first?.1 == patch)
        #expect(store.plant.commonName == "Monstera")
    }

    @Test("An empty patch never touches the network")
    @MainActor
    func emptyPatchStaysHome() async {
        let api = RecordingAPI()
        let store = PlantStoryStore(api: api, plant: .fixture())

        let failure = await store.saveEdits(PlantPatch())

        #expect(failure == nil)
        #expect(api.patches.isEmpty)
    }

    @Test("A failed edit changes nothing and hands the error back")
    @MainActor
    func failedEditKeepsThePlant() async {
        let api = RecordingAPI()
        api.failure = .offline
        let store = PlantStoryStore(api: api, plant: .fixture(commonName: "Mona"))

        var patch = PlantPatch()
        patch.commonName = "Monstera"
        let failure = await store.saveEdits(patch)

        #expect(failure == .offline)
        #expect(store.plant.commonName == "Mona")
    }

    @Test("The patch answer carries no photo, and must not erase ours")
    @MainActor
    func editKeepsThePhoto() async {
        var plant = Plant.fixture(slug: "mona")
        plant.photoURL = URL(string: "https://planty.test/photos/mona.jpg")
        let api = RecordingAPI()
        api.plantReturned = .fixture(slug: "mona", commonName: "Monstera")
        let store = PlantStoryStore(api: api, plant: plant)

        var patch = PlantPatch()
        patch.commonName = "Monstera"
        _ = await store.saveEdits(patch)

        #expect(store.plant.photoURL == plant.photoURL)
    }

    @Test("A toxicity edit replaces a detail envelope that was already loaded")
    @MainActor
    func toxicityEditReplacesLoadedDetail() async {
        let api = RecordingAPI()
        var original = Plant.fixture(slug: "mona")
        original.toxicity = Toxicity(cats: .unknown, dogs: .unknown, people: .unknown)
        api.plantReturned = original
        let store = PlantStoryStore(api: api, plant: original)
        await store.load()

        var updated = original
        updated.toxicity = Toxicity(cats: .severe, dogs: .mild, people: .mild, basis: .source)
        api.plantReturned = updated
        var patch = PlantPatch()
        patch.toxicity = updated.toxicity

        _ = await store.saveEdits(patch)

        #expect(store.toxicity?.cats == .severe)
    }

    @Test("Recorded care lands at the top of the story, note and all")
    @MainActor
    func recordInsertsTheObservation() async {
        let api = RecordingAPI()
        let store = PlantStoryStore(api: api, plant: .fixture(slug: "mona"))

        let failure = await store.record(.watered, note: "a full cup")

        #expect(failure == nil)
        #expect(api.observations.first?.0 == "mona")
        #expect(api.observations.first?.1.kind == .watered)
        #expect(api.observations.first?.1.body == "a full cup")
        #expect(store.timeline.observations.first?.body == "a full cup")
    }

    @Test("A failed care log leaves the story alone")
    @MainActor
    func failedRecordChangesNothing() async {
        let api = RecordingAPI()
        api.failure = .timedOut
        let store = PlantStoryStore(api: api, plant: .fixture())

        let failure = await store.record(.symptom, note: "yellow edges")

        #expect(failure == .timedOut)
        #expect(store.timeline.observations.isEmpty)
    }

    @Test("Recording a death archives as dead and retires the local copy")
    @MainActor
    func markDeadArchives() async {
        var plant = Plant.fixture(slug: "mona")
        plant.minTempF = 40
        let api = RecordingAPI()
        let store = PlantStoryStore(api: api, plant: plant, clock: { .reference })

        let failure = await store.markDead()

        #expect(failure == nil)
        #expect(api.archived.first?.0 == "mona")
        #expect(api.archived.first?.1 == .dead)
        #expect(store.plant.status == .dead)
        #expect(store.plant.archivedAt == .reference)
        #expect(!store.plant.canShelter, "a dead plant must drop out of the cold-shelter flow")
    }

    @Test("A failed death recording leaves the plant alive on screen")
    @MainActor
    func failedDeathStaysAlive() async {
        let api = RecordingAPI()
        api.failure = .offline
        let store = PlantStoryStore(api: api, plant: .fixture())

        let failure = await store.markDead()

        #expect(failure == .offline)
        #expect(store.plant.status == .alive)
        #expect(store.plant.archivedAt == nil)
    }

    @Test("The postmortem arrives with the asking flag honest throughout")
    @MainActor
    func postmortemArrives() async {
        let api = RecordingAPI()
        api.postmortemReturned = .fixture(lesson: "Terracotta next time.")
        let store = PlantStoryStore(api: api, plant: .fixture(slug: "mona"))

        await store.askPostmortem()

        #expect(api.postmortemAsks == ["mona"])
        #expect(store.postmortem?.lesson == "Terracotta next time.")
        #expect(store.postmortemError == nil)
        #expect(!store.isAskingPostmortem)
    }

    @Test("A postmortem that never came back says so")
    @MainActor
    func postmortemFailureIsSurfaced() async {
        let api = RecordingAPI()
        api.failure = .timedOut
        let store = PlantStoryStore(api: api, plant: .fixture())

        await store.askPostmortem()

        #expect(store.postmortem == nil)
        #expect(store.postmortemError == .timedOut)
        #expect(!store.isAskingPostmortem)
    }

    @Test("A harvest goes out with its numbers intact")
    @MainActor
    func harvestIsRecorded() async {
        let api = RecordingAPI()
        let store = PlantStoryStore(api: api, plant: .fixture(slug: "mona"), clock: { .reference })

        let failure = await store.logHarvest(quantity: 3.5, unit: "oz", notes: "first tomatoes")

        #expect(failure == nil)
        #expect(api.harvests.first?.0 == "mona")
        #expect(api.harvests.first?.1.quantity == 3.5)
        #expect(api.harvests.first?.1.unit == "oz")
        #expect(api.harvests.first?.1.notes == "first tomatoes")
        #expect(api.harvests.first?.1.occurredAt == .reference)
    }

    @Test("A failed harvest hands the error back")
    @MainActor
    func failedHarvestIsReturned() async {
        let api = RecordingAPI()
        api.failure = .offline
        let store = PlantStoryStore(api: api, plant: .fixture())

        let failure = await store.logHarvest(quantity: 1, unit: "items", notes: nil)

        #expect(failure == .offline)
    }
}
