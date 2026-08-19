import Foundation
import Testing

@testable import Planty

@Suite("Reminders store")
@MainActor
struct RemindersStoreTests {
    private func makeStore(api: FakeAPI) -> RemindersStore {
        RemindersStore(api: api, plant: .fixture())
    }

    /// One mis-tap on a paused reminder would forge an observation the
    /// service's sensor verification then reads as a clog.
    @Test("A paused reminder refuses to record")
    func markDoneRefusesWhilePaused() async {
        let api = FakeAPI()
        api.reminderList = [.fixture(kind: .misted, active: false)]
        let store = makeStore(api: api)
        await store.load()

        await store.markDone(store.reminders[0])

        #expect(api.observations.isEmpty)
        #expect(store.error == nil)
    }

    @Test("An active reminder records the observation")
    func markDoneRecordsWhileActive() async {
        let api = FakeAPI()
        api.reminderList = [.fixture(kind: .misted, active: true)]
        let store = makeStore(api: api)
        await store.load()

        await store.markDone(store.reminders[0])

        #expect(api.observations.count == 1)
        #expect(api.observations.first?.1.kind == .misted)
    }

    /// The failure comes back to the caller so a sheet can keep the draft on
    /// screen; nothing lands in the store's own error slot.
    @Test("A failed save is returned, not banked")
    func saveReturnsTheFailure() async {
        let api = FakeAPI()
        let store = makeStore(api: api)
        await store.load()
        api.failure = .timedOut

        let failure = await store.save(
            NewReminder(kind: .watered, everyDays: 3, atHours: [8], active: true, note: nil)
        )

        #expect(failure == .timedOut)
        #expect(store.error == nil)
        #expect(store.reminders.isEmpty)
    }

    @Test("A good save lands in the list and returns nil")
    func saveAppendsOnSuccess() async {
        let api = FakeAPI()
        let store = makeStore(api: api)
        await store.load()

        let failure = await store.save(
            NewReminder(kind: .watered, everyDays: 3, atHours: [8], active: true, note: nil)
        )

        #expect(failure == nil)
        #expect(store.reminders.map(\.kind) == [.watered])
    }

    /// Loading and genuinely empty must be distinguishable, or the first
    /// render claims "nothing is tracked" before anyone has been asked.
    @Test("hasLoaded flips only once an answer arrives")
    func hasLoadedDistinguishesColdFromEmpty() async {
        let api = FakeAPI()
        let store = makeStore(api: api)
        #expect(!store.hasLoaded)

        await store.load()
        #expect(store.hasLoaded)
    }

    @Test("A failed load still counts as having been asked")
    func hasLoadedSurvivesFailure() async {
        let api = FakeAPI()
        api.failure = .offline
        let store = makeStore(api: api)

        await store.load()

        #expect(store.hasLoaded)
        #expect(store.error == .offline)
    }

    /// Pause and resume have no sheet holding a draft, so their failures go
    /// through the store's own error slot.
    @Test("A failed pause banks its error")
    func setActiveBanksFailure() async {
        let api = FakeAPI()
        api.reminderList = [.fixture(kind: .misted, active: true)]
        let store = makeStore(api: api)
        await store.load()
        api.failure = .offline

        await store.setActive(store.reminders[0], to: false)

        #expect(store.error == .offline)
    }
}
