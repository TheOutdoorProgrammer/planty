import Foundation
import Testing

@testable import Planty

@MainActor
@Suite("The garden store")
struct GardenStoreTests {
    @Test("A load gathers every cross-garden record")
    func loadsGardenRecords() async {
        let api = FakeAPI()
        let question = OpenQuestion(
            id: UUID(),
            plantID: nil,
            askedOf: "Maya",
            question: "Was this watered?",
            why: nil,
            createdAt: .reference,
            status: .open
        )
        let postmortem = Postmortem(
            id: UUID(),
            plantID: UUID(),
            likelyCause: "cold",
            narrative: "It froze.",
            lesson: "Bring it in sooner.",
            createdAt: .reference
        )
        let harvest = Harvest(
            id: UUID(),
            plantID: UUID(),
            occurredAt: .reference,
            quantity: 4,
            unit: "oz",
            createdAt: .reference
        )
        api.questionList = [question]
        api.postmortemList = [postmortem]
        api.harvestList = [harvest]

        let store = GardenStore(api: api, isConfigured: true)
        await store.load()

        #expect(store.questions == [question])
        #expect(store.postmortems == [postmortem])
        #expect(store.harvests == [harvest])
        #expect(store.hasLoaded)
        #expect(store.error == nil)
    }

    @Test("Creating a question refreshes the open queue")
    func createsQuestion() async {
        let api = FakeAPI()
        api.questionList = []
        let store = GardenStore(api: api, isConfigured: true)
        let draft = NewOpenQuestion(askedOf: "Maya", question: "Is this her pot?")

        let failure = await store.createQuestion(draft)

        #expect(failure == nil)
        #expect(api.createdQuestions == [draft])
        #expect(store.questions.map(\.question) == [draft.question])
    }

    @Test("Answering removes a question from the open queue")
    func answersQuestion() async {
        let api = FakeAPI()
        let question = OpenQuestion(
            id: UUID(),
            plantID: nil,
            askedOf: "self",
            question: "Did it rain?",
            why: nil,
            createdAt: .reference,
            status: .open
        )
        api.questionList = [question]
        let store = GardenStore(api: api, isConfigured: true)
        await store.load()

        let failure = await store.answer(question, with: "Yes")

        #expect(failure == nil)
        #expect(api.answeredQuestions.first?.1 == "Yes")
        #expect(store.questions.isEmpty)
    }

    @Test("Planning a trip and checking cold retain the service answers")
    func plansAndChecks() async {
        let api = FakeAPI()
        api.plantList = [.fixture(commonName: "Mona")]
        let store = GardenStore(api: api, isConfigured: true)
        let trip = NewAwayPeriod(startsAt: .reference, endsAt: .reference.addingTimeInterval(86_400))

        #expect(await store.planAway(trip) == nil)
        #expect(await store.checkCold(forecastLowF: 37) == nil)
        #expect(store.plannedAway?.startsAt == trip.startsAt)
        #expect(store.coldWatch?.plants.map(\.commonName) == ["Mona"])
    }

    @Test("Changing servers clears cross-server history")
    func replaceClearsRecords() async {
        let api = FakeAPI()
        api.harvestList = [
            Harvest(
                id: UUID(),
                plantID: UUID(),
                occurredAt: .reference,
                quantity: 1,
                unit: "lb",
                createdAt: .reference
            )
        ]
        let store = GardenStore(api: api, isConfigured: true)
        await store.load()

        store.replace(api: FakeAPI(), isConfigured: false)

        #expect(store.harvests.isEmpty)
        #expect(!store.hasLoaded)
        #expect(!store.isConfigured)
    }
}
