import Foundation
import Testing

@testable import Planty

/// Asking about a plant needs no photograph, which is the whole point: a
/// diagnosis refuses without one, and most questions are not about a picture.
@Suite("Asking about a plant")
struct ConsultStoreTests {
    @Test("A question goes out with no photo and a durable conversation ID")
    @MainActor
    func firstQuestionOpensAConversation() async {
        let api = FakeAPI()
        let store = ConsultStore(api: api, plant: .fixture())

        store.composer = "does this need water?"
        await store.send()

        #expect(api.asked.count == 1)
        #expect(api.asked.first?.1.message == "does this need water?")
        #expect(api.asked.first?.1.conversationID != nil)
        #expect(store.messages.count == 2, "the question and its answer should both be on screen")
    }

    /// Without this every follow-up is answered by a model that has never seen
    /// the exchange above it.
    @Test("A follow-up carries the conversation it belongs to")
    @MainActor
    func followUpsStayInTheSameConversation() async {
        let api = FakeAPI()
        let store = ConsultStore(api: api, plant: .fixture())

        await store.send("first")
        await store.send("second")

        #expect(api.asked.count == 2)
        #expect(api.asked.last?.1.conversationID == api.asked.first?.1.conversationID)
    }

    @Test("A saved conversation reopens with its transcript and continues")
    @MainActor
    func savedConversationResumes() async {
        let conversationID = UUID()
        let api = FakeAPI()
        api.answer = .fixture(conversationID: conversationID)
        let turn = PlantConversationTurn(
            id: UUID(),
            conversationID: conversationID,
            asked: "What caused this spot?",
            reply: "It looks like old mechanical damage.",
            confidence: 0.8,
            lookedAt: "photo.jpg",
            suggestedFollowUps: [],
            steps: [],
            photoID: UUID(),
            createdAt: .reference
        )
        let store = ConsultStore(
            api: api,
            plant: .fixture(),
            conversation: PlantConversation(id: conversationID, turns: [turn])
        )

        #expect(store.messages.count == 2)
        #expect(store.messages.first?.text == "What caused this spot?")
        #expect(store.messages.first?.photoID == turn.photoID)

        await store.send("Has it changed since then?")

        #expect(api.asked.last?.1.conversationID == conversationID)
        #expect(store.messages.count == 4)
    }

    @Test("A pending conversation finishes after the app reopens")
    @MainActor
    func pendingConversationResumes() async {
        let conversationID = UUID()
        let turnID = UUID()
        let api = FakeAPI()
        let pending = PlantConversationTurn(
            id: turnID,
            conversationID: conversationID,
            asked: "How is it doing?",
            reply: nil,
            confidence: 0,
            lookedAt: nil,
            suggestedFollowUps: [],
            steps: [],
            photoID: nil,
            status: .processing,
            createdAt: .reference
        )
        let completed = PlantConversationTurn(
            id: turnID,
            conversationID: conversationID,
            asked: pending.asked,
            reply: "It is healthy and does not need attention today.",
            confidence: 0.9,
            lookedAt: nil,
            suggestedFollowUps: [],
            steps: [],
            photoID: nil,
            createdAt: .reference
        )
        api.conversationResponses = [
            PlantConversation(id: conversationID, turns: [completed])
        ]
        let store = ConsultStore(
            api: api,
            plant: .fixture(),
            conversation: PlantConversation(id: conversationID, turns: [pending]),
            pollInterval: .milliseconds(1)
        )

        #expect(store.isThinking)
        #expect(store.messages.map(\.text) == [pending.asked])

        await store.begin()

        #expect(api.conversationReads == 1)
        #expect(!store.isThinking)
        #expect(store.messages.map(\.text) == [pending.asked, completed.reply])
    }

    @Test("An empty question is not sent")
    @MainActor
    func blankQuestionsAreIgnored() async {
        let api = FakeAPI()
        let store = ConsultStore(api: api, plant: .fixture())

        store.composer = "   "
        await store.send()

        #expect(api.asked.isEmpty)
        #expect(store.messages.isEmpty)
    }

    @Test("Sending clears the composer so the question is not asked twice")
    @MainActor
    func composerClears() async {
        let store = ConsultStore(api: FakeAPI(), plant: .fixture())

        store.composer = "how is it doing?"
        await store.send()

        #expect(store.composer.isEmpty)
    }

    /// The reply says whether it opened a photograph, and that only means
    /// something if the app shows it when it did and stays quiet when it did not.
    @Test("Whether it looked at a photo is reported honestly")
    @MainActor
    func looksAreReported() async {
        let api = FakeAPI()
        api.answer = .fixture(lookedAt: "day-01.jpg (18 August)")
        let looked = ConsultStore(api: api, plant: .fixture())
        await looked.send("what colour are the leaves?")

        #expect(looked.messages.last?.answer?.didOpenAPhotograph == true)

        let quiet = FakeAPI()
        quiet.answer = .fixture(lookedAt: "")
        let didNotLook = ConsultStore(api: quiet, plant: .fixture())
        await didNotLook.send("when did I water it?")

        #expect(didNotLook.messages.last?.answer?.didOpenAPhotograph == false)
    }

    /// A question left on screen with nothing under it reads as a reply still
    /// coming, so a failure has to be said out loud.
    @Test("A failure is surfaced rather than left hanging")
    @MainActor
    func failuresAreShown() async {
        let api = FakeAPI()
        api.failure = .offline
        let store = ConsultStore(api: api, plant: .fixture())

        await store.send("anything")

        #expect(store.error == .offline)
        #expect(!store.isThinking)
    }

    @Test("A cancelled request leaves its durable question ready to resume")
    @MainActor
    func cancellationRestoresDraft() async {
        let api = FakeAPI()
        api.failure = PlantyError.from(URLError(.cancelled))
        let store = ConsultStore(api: api, plant: .fixture())

        await store.send("anything")

        #expect(store.error == nil)
        #expect(store.messages.count == 1)
        #expect(store.composer.isEmpty)
        #expect(store.isThinking)
        #expect(store.failed == nil)
    }

    @Test("A Today question carries the card without rewriting the chat bubble")
    @MainActor
    func todayFindingContextIsHiddenFromTheTranscript() async {
        let api = FakeAPI()
        let plant = Plant.fixture(commonName: "Mona")
        let entry = todayEntry(for: plant)
        let store = ConsultStore(
            api: api,
            plant: plant,
            origin: .todayFinding(entry)
        )
        let question = "Can this wait until tomorrow?"

        await store.send(question)

        let sent = api.asked.first?.1.message ?? ""
        #expect(sent.contains("<today_finding>"))
        #expect(sent.contains("Recommended action: \(entry.verdict.action.instruction)"))
        #expect(sent.contains("Evidence summary: Moisture stayed below the calibrated range"))
        #expect(sent.contains("User's question:\n\(question)"))
        #expect(store.messages.first?.text == question)
        #expect(store.openingTitle == "Ask about today's finding.")
        #expect(store.openers.first == "Why is Planty recommending this?")
    }

    @Test("A Today finding is not repeated after the conversation starts")
    @MainActor
    func todayFindingContextIsOnlySentOnce() async {
        let api = FakeAPI()
        let plant = Plant.fixture()
        let store = ConsultStore(
            api: api,
            plant: plant,
            origin: .todayFinding(todayEntry(for: plant))
        )

        await store.send("Why this recommendation?")
        await store.send("What should I watch for next?")

        #expect(api.asked.count == 2)
        #expect(api.asked.first?.1.message.contains("<today_finding>") == true)
        #expect(api.asked.last?.1.message == "What should I watch for next?")
        #expect(api.asked.last?.1.conversationID == api.asked.first?.1.conversationID)
    }
}

/// Retyping what you just said is the worst possible response to a failure,
/// and the composer is cleared the moment a question is sent.
@Suite("Recovering a failed question")
struct ConsultRecoveryTests {
    @Test("A failed question can be asked again without retyping it")
    @MainActor
    func retryReusesTheQuestion() async {
        let api = FakeAPI()
        api.failure = .offline
        let store = ConsultStore(api: api, plant: .fixture())

        store.composer = "is this too wet?"
        await store.send()

        #expect(store.failed?.text == "is this too wet?")
        #expect(store.messages.count == 1, "the unanswered question is still on screen")

        api.failure = nil
        await store.retry()

        #expect(store.error == nil)
        #expect(store.failed == nil)
        // The fake only records what it answered, so one entry means the retry
        // carried the original words rather than an empty string.
        #expect(api.asked.map(\.1.message) == ["is this too wet?"])
        #expect(store.messages.count == 2, "the dangling question was left behind")
    }

    @Test("The words come back to the composer to be edited")
    @MainActor
    func draftIsRecoverable() async {
        let api = FakeAPI()
        api.failure = .offline
        let store = ConsultStore(api: api, plant: .fixture())

        store.composer = "why are the leaves curling"
        await store.send()
        store.recoverDraft()

        #expect(store.composer == "why are the leaves curling")
        #expect(store.messages.isEmpty)
        #expect(store.error == nil)
    }

    /// Tapping a suggestion while something is half-typed must not eat it.
    @Test("A suggestion does not destroy a half-typed question")
    @MainActor
    func suggestionsKeepTheDraftOnFailure() async {
        let api = FakeAPI()
        api.failure = .offline
        let store = ConsultStore(api: api, plant: .fixture())

        store.composer = "half a thought"
        await store.send("Does this need water?")

        #expect(store.composer == "half a thought")
    }
}

private func todayEntry(for plant: Plant) -> DigestEntry {
    DigestEntry(
        plant: plant,
        verdict: Verdict(
            id: UUID(),
            plantID: plant.id,
            forDate: .reference,
            action: .water,
            reasoning: "The soil has stayed dry for two days.",
            confidence: 0.82,
            evidence: Evidence(
                readingIDs: [UUID(), UUID()],
                observationIDs: [UUID()],
                photoIDs: [UUID()],
                sensorSummary: "Moisture stayed below the calibrated range for two days.",
                modelVersion: "test-model"
            ),
            createdAt: .reference,
            acknowledgedAt: nil
        ),
        risk: 3
    )
}
