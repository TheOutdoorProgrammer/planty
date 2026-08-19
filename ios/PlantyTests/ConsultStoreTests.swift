import Foundation
import Testing

@testable import Planty

/// Asking about a plant needs no photograph, which is the whole point: a
/// diagnosis refuses without one, and most questions are not about a picture.
@Suite("Asking about a plant")
struct ConsultStoreTests {
    @Test("A question goes out with no photo and no conversation")
    @MainActor
    func firstQuestionOpensAConversation() async {
        let api = FakeAPI()
        let store = ConsultStore(api: api, plant: .fixture())

        store.composer = "does this need water?"
        await store.send()

        #expect(api.asked.count == 1)
        #expect(api.asked.first?.1.message == "does this need water?")
        #expect(api.asked.first?.1.conversationID == nil, "the first question resumed something")
        #expect(store.messages.count == 2, "the question and its answer should both be on screen")
    }

    /// Without this every follow-up is answered by a model that has never seen
    /// the exchange above it.
    @Test("A follow-up carries the conversation it belongs to")
    @MainActor
    func followUpsStayInTheSameConversation() async {
        let conversation = UUID()
        let api = FakeAPI()
        api.answer = .fixture(conversationID: conversation)
        let store = ConsultStore(api: api, plant: .fixture())

        await store.send("first")
        await store.send("second")

        #expect(api.asked.count == 2)
        #expect(api.asked.last?.1.conversationID == conversation)
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

    @Test("A cancelled question is not an error")
    @MainActor
    func cancellationIsQuiet() async {
        let api = FakeAPI()
        api.failure = PlantyError.from(URLError(.cancelled))
        let store = ConsultStore(api: api, plant: .fixture())

        await store.send("anything")

        #expect(store.error == nil)
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

        #expect(store.failed == "is this too wet?")
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
