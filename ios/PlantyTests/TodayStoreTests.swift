import Foundation
import Testing

@testable import Planty

/// Records what was asked of it and answers whatever the test set.
final class FakeAPI: PlantyAPI, ReminderResolving, @unchecked Sendable {
    private let lock = NSLock()
    private var _digest: Digest = .fixture()
    private var _plants: [Plant] = [.fixture()]
    private var _failure: PlantyError?
    private var _failAcknowledge = false
    private var _acknowledged: [UUID] = []
    private var _observations: [(String, NewObservation)] = []

    var digest: Digest {
        get { lock.withLock { _digest } }
        set { lock.withLock { _digest = newValue } }
    }

    var plantList: [Plant] {
        get { lock.withLock { _plants } }
        set { lock.withLock { _plants = newValue } }
    }

    var failure: PlantyError? {
        get { lock.withLock { _failure } }
        set { lock.withLock { _failure = newValue } }
    }

    /// Acknowledging can fail on its own: the observation writes, the verdict
    /// does not settle, and the service keeps chasing.
    var failAcknowledge: Bool {
        get { lock.withLock { _failAcknowledge } }
        set { lock.withLock { _failAcknowledge = newValue } }
    }

    var acknowledged: [UUID] { lock.withLock { _acknowledged } }
    var observations: [(String, NewObservation)] { lock.withLock { _observations } }

    private var _answer: PlantAnswer = .fixture()
    private var _asked: [(String, PlantQuestion)] = []
    private var _enqueuedStatus: ConversationTurnStatus = .complete
    private var _conversationResponses: [PlantConversation] = []
    private var _conversationReads = 0
    private var _reminders: [Reminder] = []
    private var _notes: [PlantNote] = []
    private var _detailVerdict: Verdict?
    private var _household: [PlantNote] = []
    private var _answered: [(UUID, String)] = []
    private var _scratchAsks: [ScratchQuestion] = []
    private var _created: [String] = []
    private var _uploads: [(String, Data)] = []
    private var _questions: [OpenQuestion]?
    private var _postmortems: [Postmortem] = []
    private var _harvests: [Harvest] = []
    private var _createdQuestions: [NewOpenQuestion] = []
    private var _pushRegistrations: [PushDeviceRegistration] = []
    private var _pushTests: [PushInstallationRequest] = []
    private var _reminderResolutions: [(UUID, Date, ReminderDisposition, String, UUID)] = []

    var answer: PlantAnswer {
        get { lock.withLock { _answer } }
        set { lock.withLock { _answer = newValue } }
    }

    var asked: [(String, PlantQuestion)] { lock.withLock { _asked } }
    var scratchAsks: [ScratchQuestion] { lock.withLock { _scratchAsks } }
    var conversationReads: Int { lock.withLock { _conversationReads } }

    var enqueuedStatus: ConversationTurnStatus {
        get { lock.withLock { _enqueuedStatus } }
        set { lock.withLock { _enqueuedStatus = newValue } }
    }

    var conversationResponses: [PlantConversation] {
        get { lock.withLock { _conversationResponses } }
        set { lock.withLock { _conversationResponses = newValue } }
    }

    /// Every way a plant or a photo can come into existence, so a chat that
    /// promises to create nothing can be held to it.
    var created: [String] { lock.withLock { _created } }
    var uploads: [(String, Data)] { lock.withLock { _uploads } }

    var questionList: [OpenQuestion] {
        get { lock.withLock { _questions ?? _digest.openQuestions } }
        set { lock.withLock { _questions = newValue } }
    }

    var postmortemList: [Postmortem] {
        get { lock.withLock { _postmortems } }
        set { lock.withLock { _postmortems = newValue } }
    }

    var harvestList: [Harvest] {
        get { lock.withLock { _harvests } }
        set { lock.withLock { _harvests = newValue } }
    }

    var createdQuestions: [NewOpenQuestion] { lock.withLock { _createdQuestions } }
    var pushRegistrations: [PushDeviceRegistration] { lock.withLock { _pushRegistrations } }
    var pushTests: [PushInstallationRequest] { lock.withLock { _pushTests } }
    var reminderResolutions: [(UUID, Date, ReminderDisposition, String, UUID)] {
        lock.withLock { _reminderResolutions }
    }

    var reminderList: [Reminder] {
        get { lock.withLock { _reminders } }
        set { lock.withLock { _reminders = newValue } }
    }

    var noteList: [PlantNote] {
        get { lock.withLock { _notes } }
        set { lock.withLock { _notes = newValue } }
    }

    var answeredQuestions: [(UUID, String)] { lock.withLock { _answered } }

    var householdList: [PlantNote] {
        get { lock.withLock { _household } }
        set { lock.withLock { _household = newValue } }
    }

    /// What GET /v1/plants/{slug} reports as the plant's open verdict.
    var detailVerdict: Verdict? {
        get { lock.withLock { _detailVerdict } }
        set { lock.withLock { _detailVerdict = newValue } }
    }

    private func check() throws {
        if let failure { throw failure }
    }

    func today() async throws -> Digest {
        try check()
        return digest
    }

    func plants(filter: PlantFilter) async throws -> [Plant] {
        try check()
        return plantList
    }

    func plant(slug: String) async throws -> PlantDetail {
        try check()
        guard let plant = plantList.first(where: { $0.slug == slug }) else {
            throw PlantyError.notFound
        }
        return PlantDetail(plant: plant, verdict: detailVerdict)
    }

    func createPlant(_ draft: NewPlant) async throws -> Plant {
        try check()
        lock.withLock { _created.append(draft.commonName) }
        return .fixture(commonName: draft.commonName)
    }

    func updatePlant(slug: String, patch: PlantPatch) async throws -> Plant {
        try check()
        return .fixture(slug: slug)
    }

    func archivePlant(slug: String, status: PlantStatus) async throws {
        try check()
    }

    func addObservation(slug: String, observation: NewObservation) async throws -> PlantObservation {
        try check()
        lock.withLock { _observations.append((slug, observation)) }
        return PlantObservation(
            id: UUID(),
            plantID: UUID(),
            kind: observation.kind,
            occurredAt: observation.occurredAt,
            source: observation.source,
            createdAt: observation.occurredAt
        )
    }

    func timeline(slug: String) async throws -> PlantTimeline {
        try check()
        return PlantTimeline()
    }

    func uploadPhoto(slug: String, jpeg: Data, caption: String?, takenAt: Date) async throws -> Photo {
        try check()
        lock.withLock { _uploads.append((slug, jpeg)) }
        return Photo(
            id: UUID(),
            plantID: UUID(),
            storageKey: "photos/\(slug).jpg",
            takenAt: takenAt,
            createdAt: takenAt
        )
    }

    func acknowledge(verdictID: UUID) async throws {
        try check()
        if failAcknowledge { throw PlantyError.timedOut }
        lock.withLock { _acknowledged.append(verdictID) }
    }

    func sensors() async throws -> [SensorLink] {
        try check()
        return []
    }

    func shelter(slugs: [String], indoors: Bool) async throws -> Int {
        try check()
        return slugs.count
    }

    func identify(jpeg: Data, metadata: CaptureMetadata) async throws -> [IdentificationCandidate] {
        try check()
        return [IdentificationCandidate(commonName: "Stub", scientificName: nil, confidence: 0.5)]
    }

    func calibrate(sensorID: UUID, to calibration: SensorCalibration) async throws -> SensorLink {
        try check()
        return SensorLink(
            id: sensorID,
            haEntityID: "sensor.stub",
            role: .soilMoisture,
            dryBaseline: calibration.dryBaseline,
            wetBaseline: calibration.wetBaseline,
            calibratedAt: Date(),
            createdAt: Date()
        )
    }

    func logHarvest(_ harvest: NewHarvest, on slug: String) async throws -> Harvest {
        try check()
        return Harvest(
            id: UUID(),
            plantID: UUID(),
            occurredAt: harvest.occurredAt,
            quantity: harvest.quantity,
            unit: harvest.unit,
            createdAt: harvest.occurredAt
        )
    }

    func ask(slug: String, question: PlantQuestion) async throws -> PlantAnswer {
        try check()
        lock.withLock { _asked.append((slug, question)) }
        return lock.withLock { _answer }
    }

    func enqueueMessage(
        slug: String,
        conversationID: UUID,
        message: ConversationMessage
    ) async throws -> PlantConversationTurn {
        try check()
        return lock.withLock {
            _asked.append((
                slug,
                PlantQuestion(
                    message: message.message,
                    photo: message.photo,
                    conversationID: conversationID
                )
            ))
            let complete = _enqueuedStatus == .complete
            return PlantConversationTurn(
                id: message.id,
                conversationID: conversationID,
                asked: message.message,
                reply: complete ? _answer.reply : nil,
                confidence: complete ? _answer.confidence : 0,
                lookedAt: complete ? _answer.lookedAt : nil,
                suggestedFollowUps: complete ? _answer.suggestedFollowUps : [],
                steps: complete ? _answer.steps : [],
                photoID: nil,
                status: _enqueuedStatus,
                createdAt: Date()
            )
        }
    }

    func conversation(slug: String, id: UUID) async throws -> PlantConversation {
        try check()
        return try lock.withLock {
            _conversationReads += 1
            guard !_conversationResponses.isEmpty else { throw PlantyError.notFound }
            return _conversationResponses.removeFirst()
        }
    }

    func ask(_ question: ScratchQuestion) async throws -> PlantAnswer {
        try check()
        lock.withLock { _scratchAsks.append(question) }
        return lock.withLock { _answer }
    }

    func reminders(slug: String) async throws -> [Reminder] {
        try check()
        return reminderList
    }

    func notes(slug: String) async throws -> [PlantNote] {
        try check()
        return noteList
    }


    func answerQuestion(id: UUID, answer: String) async throws {
        try check()
        lock.withLock {
            _answered.append((id, answer))
            if let index = _questions?.firstIndex(where: { $0.id == id }) {
                _questions?[index].answer = answer
                _questions?[index].status = .answered
                _questions?[index].answeredAt = Date()
            }
        }
    }
    func householdNotes() async throws -> [PlantNote] {
        try check()
        return householdList
    }

    func addHouseholdNote(draft: NoteDraft) async throws -> PlantNote {
        try check()
        let written = PlantNote.fixture(title: draft.title, body: draft.body ?? "")
        householdList.insert(written, at: 0)
        return written
    }

    func addNote(slug: String, draft: NoteDraft) async throws -> PlantNote {
        try check()
        let written = PlantNote.fixture(title: draft.title, body: draft.body ?? "")
        noteList.insert(written, at: 0)
        return written
    }

    func updateNote(id: UUID, draft: NoteDraft) async throws -> PlantNote {
        try check()
        guard let index = noteList.firstIndex(where: { $0.id == id }) else {
            throw PlantyError.offline
        }
        let existing = noteList[index]
        let changed = PlantNote(
            id: existing.id,
            plantID: existing.plantID,
            title: draft.title ?? existing.title,
            body: draft.body ?? existing.body,
            createdAt: existing.createdAt,
            updatedAt: existing.createdAt.addingTimeInterval(60)
        )
        noteList[index] = changed
        return changed
    }

    func deleteNote(id: UUID) async throws {
        try check()
        noteList.removeAll { $0.id == id }
    }

    func setReminder(slug: String, reminder: NewReminder) async throws -> Reminder {
        try check()
        let saved = Reminder(
            id: UUID(),
            plantID: UUID(),
            kind: reminder.kind,
            everyDays: reminder.everyDays,
            atHours: reminder.atHours,
            active: reminder.active,
            note: reminder.note,
            lastDone: nil,
            due: nil
        )
        lock.withLock {
            _reminders.removeAll { $0.kind == saved.kind }
            _reminders.append(saved)
        }
        return saved
    }

    func deleteReminder(slug: String, kind: ObservationKind) async throws {
        try check()
        lock.withLock { _reminders.removeAll { $0.kind == kind } }
    }

    func resolveReminder(
        reminderID: UUID,
        dueAt: Date,
        disposition: ReminderDisposition,
        note: String,
        idempotencyKey: UUID
    ) async throws -> ReminderResolutionResult {
        try check()
        lock.withLock {
            _reminderResolutions.append((reminderID, dueAt, disposition, note, idempotencyKey))
        }
        let observation: PlantObservation?
        if disposition == .completed {
            let due = digest.dueReminders.first {
                $0.reminder.id == reminderID && $0.dueAt == dueAt
            }
            if let due {
                let draft = NewObservation(kind: due.reminder.kind, body: due.reminder.note)
                lock.withLock { _observations.append((due.plant.slug, draft)) }
                observation = PlantObservation(
                    id: UUID(),
                    plantID: due.plant.id,
                    kind: draft.kind,
                    body: draft.body,
                    occurredAt: draft.occurredAt,
                    source: draft.source,
                    createdAt: draft.occurredAt
                )
            } else {
                observation = nil
            }
        } else {
            observation = nil
        }
        return ReminderResolutionResult(
            idempotencyKey: idempotencyKey,
            reminderID: reminderID,
            dueAt: dueAt,
            disposition: disposition,
            note: note,
            observation: observation,
            respondedAt: .reference
        )
    }

    func createPlantFromPhoto(_ ask: PlantFromPhoto) async throws -> PlantFromPhotoResult {
        try check()
        let name = ask.commonName ?? "Stub"
        lock.withLock { _created.append(name) }
        return PlantFromPhotoResult(
            plant: .fixture(commonName: name),
            candidates: [],
            photoError: nil
        )
    }

    func linkSensor(_ link: NewSensorLink) async throws -> SensorLink {
        try check()
        return SensorLink(
            id: UUID(),
            plantID: link.plantID,
            zone: link.zone,
            haEntityID: link.haEntityID,
            role: link.role,
            createdAt: Date()
        )
    }

    func postmortem(slug: String) async throws -> Postmortem {
        try check()
        return Postmortem(
            id: UUID(),
            plantID: UUID(),
            likelyCause: "overwatering",
            narrative: "It stayed wet for weeks.",
            lesson: "Let the top inch dry first.",
            createdAt: Date()
        )
    }

    func postmortems() async throws -> [Postmortem] {
        try check()
        return postmortemList
    }

    func harvests(slug: String?) async throws -> [Harvest] {
        try check()
        return harvestList
    }

    func questions(status: QuestionStatus) async throws -> [OpenQuestion] {
        try check()
        return questionList.filter { ($0.status ?? .open) == status }
    }

    func createQuestion(_ draft: NewOpenQuestion) async throws -> OpenQuestion {
        try check()
        let created = OpenQuestion(
            id: UUID(),
            plantID: draft.plantID,
            askedOf: draft.askedOf ?? Plant.stewardSelf,
            question: draft.question,
            why: draft.why,
            createdAt: Date(),
            status: .open
        )
        lock.withLock {
            _createdQuestions.append(draft)
            if _questions == nil { _questions = _digest.openQuestions }
            _questions?.insert(created, at: 0)
        }
        return created
    }

    func planAway(_ draft: NewAwayPeriod) async throws -> AwayPeriod {
        try check()
        return AwayPeriod(
            id: UUID(),
            startsAt: draft.startsAt,
            endsAt: draft.endsAt,
            backupContact: draft.backupContact,
            backupNotify: draft.backupNotify,
            note: draft.note,
            createdAt: Date()
        )
    }

    func coldWatch(forecastLowF: Double) async throws -> ColdWatch {
        try check()
        return ColdWatch(forecastLowF: forecastLowF, plants: plantList)
    }

    func health() async throws {
        try check()
    }

    func registerPushDevice(_ device: PushDeviceRegistration) async throws -> PushRegistrationReceipt {
        try check()
        lock.withLock { _pushRegistrations.append(device) }
        return PushRegistrationReceipt(
            environment: device.environment,
            installationID: device.installationID,
            acceptedAt: .reference
        )
    }

    func pushHealth(installationID: UUID, environment: String) async throws -> PushHealth {
        try check()
        let registered = lock.withLock {
            _pushRegistrations.last {
                $0.installationID == installationID && $0.environment == environment
            }
        }
        return PushHealth(
            server: PushServerStatus(
                configured: true,
                environment: environment,
                bundleID: "zone.stout.Planty"
            ),
            registration: registered.map {
                PushRegistrationReceipt(
                    environment: $0.environment,
                    installationID: $0.installationID,
                    acceptedAt: .reference
                )
            }
        )
    }

    func testPush(_ request: PushInstallationRequest) async throws {
        try check()
        lock.withLock { _pushTests.append(request) }
    }
}

@MainActor
@Suite("The Today store")
struct TodayStoreTests {
    private func store(_ api: FakeAPI) -> TodayStore {
        TodayStore(api: api, isConfigured: true, clock: { .reference })
    }

    @Test("A load with fresh empty evidence presents as calm")
    func loadsCalm() async {
        let api = FakeAPI()
        api.digest = .fixture(date: Date.reference.minus(hours: 1), checked: 4)
        api.plantList = (0..<4).map { .fixture(slug: "p\($0)") }

        let store = store(api)
        await store.load()

        guard case .calm(let summary) = store.presentation else {
            Issue.record("expected calm, got \(store.presentation)")
            return
        }
        #expect(summary.checked == 4)
    }

    @Test("A failed load presents the error, never calm")
    func loadFailure() async {
        let api = FakeAPI()
        api.failure = .timedOut

        let store = store(api)
        await store.load()

        #expect(store.presentation == .failed(error: .timedOut, cached: nil))
    }

    @Test("Not now hides the card but never acknowledges it")
    func postponeHidesWithoutResolving() async {
        let api = FakeAPI()
        let entry = DigestEntry.fixture()
        api.digest = .fixture(date: Date.reference.minus(hours: 1), entries: [entry])

        let store = store(api)
        await store.load()
        store.postpone(entry, by: .anHour)

        #expect(api.acknowledged.isEmpty)
        guard case .calm = store.presentation else {
            Issue.record("a postponed card should leave nothing to do")
            return
        }
    }

    @Test("Hiding a card does not fake the freshness count")
    func hidingKeepsCheckedHonest() async {
        let api = FakeAPI()
        let entry = DigestEntry.fixture()
        api.digest = .fixture(date: Date.reference.minus(hours: 1), entries: [entry], checked: 9)

        let store = store(api)
        await store.load()
        store.postpone(entry, by: .anHour)

        guard case .calm(let summary) = store.presentation else {
            Issue.record("expected calm")
            return
        }
        #expect(summary.checked == 9)
    }

    @Test("Completing records what happened and then acknowledges")
    func completeRecordsFirst() async {
        let api = FakeAPI()
        let entry = DigestEntry.fixture()
        api.digest = .fixture(date: Date.reference.minus(hours: 1), entries: [entry])

        let store = store(api)
        await store.load()
        await store.complete(entry, kind: .watered)

        #expect(api.observations.count == 1)
        #expect(api.observations.first?.1.kind == .watered)
        #expect(api.acknowledged == [entry.verdict.id])
    }

    @Test("A stale digest with a hidden card is still stale, not calm")
    func hidingCannotManufactureCalm() async {
        let api = FakeAPI()
        let entry = DigestEntry.fixture()
        api.digest = .fixture(date: Date.reference.minus(days: 4), entries: [entry])

        let store = store(api)
        await store.load()
        store.postpone(entry, by: .anHour)

        guard case .stale = store.presentation else {
            Issue.record("hiding a card must not turn stale evidence into calm")
            return
        }
    }

    @Test("Changing the service throws away every cached answer")
    func replaceClearsCache() async {
        let api = FakeAPI()
        let store = store(api)
        await store.load()
        #expect(store.digest != nil)

        store.replace(api: FakeAPI(), isConfigured: false)
        #expect(store.digest == nil)
        #expect(store.presentation == .unconfigured)
    }
}

@MainActor
@Suite("The capture store")
struct CaptureStoreTests {
    @Test("A failed save keeps the photo on screen")
    func failureKeepsThePhoto() async {
        let api = FakeAPI()
        api.failure = .offline
        let store = CaptureStore(api: api, selectedPlant: .fixture())

        store.accept(jpeg: Data([0xFF, 0xD8]))
        await store.save(recording: .watered)

        guard case .failed(let photo, let action, let error) = store.stage else {
            Issue.record("a failed upload must not discard the image")
            return
        }
        #expect(photo.jpeg == Data([0xFF, 0xD8]))
        #expect(error == .offline)
        #expect(action == .save(.watered),
                "a retry would have saved the photo and dropped the watering")
    }

    @Test("A successful save uploads the photo, then the exception tag")
    func savesPhotoThenObservation() async {
        let api = FakeAPI()
        let store = CaptureStore(api: api, selectedPlant: .fixture(slug: "mona"))

        store.accept(jpeg: Data([0xFF, 0xD8]))
        store.note = "  lower leaves  "
        await store.save(recording: .repotted)

        #expect(store.stage == .ready)
        #expect(api.observations.first?.0 == "mona")
        #expect(api.observations.first?.1.kind == .repotted)
        #expect(api.observations.first?.1.body == "lower leaves")
        #expect(store.toast?.contains("Mona") == true)
    }

    @Test("Save photo only records no observation at all")
    func photoOnly() async {
        let api = FakeAPI()
        let store = CaptureStore(api: api, selectedPlant: .fixture())

        store.accept(jpeg: Data([0xFF]))
        await store.save(recording: nil)

        #expect(api.observations.isEmpty)
        #expect(store.stage == .ready)
    }

    @Test("Nothing is sent while no plant is chosen")
    func requiresAPlant() async {
        let api = FakeAPI()
        let store = CaptureStore(api: api)

        store.accept(jpeg: Data([0xFF]))
        await store.save(recording: .watered)

        #expect(api.observations.isEmpty)
        #expect(store.stage.photo != nil)
    }
}
