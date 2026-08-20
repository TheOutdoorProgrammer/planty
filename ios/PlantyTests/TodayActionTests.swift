import Foundation
import Testing

@testable import Planty

@MainActor
@Suite("Today action flow")
struct TodayActionTests {
    private func store(_ api: FakeAPI) -> TodayStore {
        TodayStore(api: api, isConfigured: true, clock: { .reference })
    }

    @Test("Today keeps the live-list photo on the judged digest plant")
    func digestKeepsLatestPhoto() async {
        let plantID = UUID()
        let digestPlant = Plant.fixture(slug: "fern", commonName: "Fern", id: plantID)
        let entry = DigestEntry.fixture(plant: digestPlant, action: .check)

        var listed = Plant.fixture(slug: "fern", commonName: "Fern", id: plantID)
        listed.photoURL = URL(string: "https://example.com/fern.jpg")
        listed.photoTakenAt = .reference

        let api = FakeAPI()
        api.digest = .fixture(
            date: Date.reference.minus(hours: 1),
            entries: [entry],
            checked: 1
        )
        api.plantList = [listed]

        let subject = store(api)
        await subject.load()

        let loaded = subject.digest?.sortedEntries.first?.plant
        #expect(loaded?.photoURL == listed.photoURL)
        #expect(loaded?.photoTakenAt == listed.photoTakenAt)
    }

    @Test("Acknowledging a Watch item clears it without inventing care")
    func acknowledgementClearsWatch() async {
        let entry = DigestEntry.fixture(action: .check)
        let api = FakeAPI()
        api.digest = .fixture(
            date: Date.reference.minus(hours: 1),
            entries: [entry],
            checked: 1
        )
        api.plantList = [entry.plant]

        let subject = store(api)
        await subject.load()

        let failure = await subject.acknowledge(entry)

        #expect(failure == nil)
        #expect(api.acknowledged == [entry.verdict.id])
        #expect(api.observations.isEmpty)
        guard case .calm = subject.presentation else {
            Issue.record("an acknowledged Watch card should disappear immediately")
            return
        }
    }

    @Test("An optional photo is evidence only and does not clear the card")
    func photoDoesNotAcknowledge() async {
        let entry = DigestEntry.fixture(action: .check)
        let api = FakeAPI()
        api.digest = .fixture(
            date: Date.reference.minus(hours: 1),
            entries: [entry],
            checked: 1
        )
        api.plantList = [entry.plant]

        let subject = store(api)
        await subject.load()

        let jpeg = Data([0xFF, 0xD8, 0xFF, 0xD9])
        let failure = await subject.addPhoto(entry, jpeg: jpeg)

        #expect(failure == nil)
        #expect(api.uploads.count == 1)
        #expect(api.uploads.first?.0 == entry.plant.slug)
        #expect(api.acknowledged.isEmpty)
        guard case .actions = subject.presentation else {
            Issue.record("adding a photo alone must leave the Today card in place")
            return
        }
    }
}
