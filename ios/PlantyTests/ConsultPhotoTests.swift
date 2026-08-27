import Foundation
import Testing

@testable import Planty

private let jpeg = Data([0xFF, 0xD8, 0xFF, 0xE0, 0x01])
private let otherJPEG = Data([0xFF, 0xD8, 0xFF, 0xE1, 0x02])

/// Answering "show me the underside of a leaf" needs a way to send one, in
/// every chat, on any turn.
@Suite("Adding a photo to a message")
struct ConsultPhotoTests {
    @Test("An attached photo rides along with the question")
    @MainActor
    func photoGoesWithTheQuestion() async {
        let api = FakeAPI()
        let store = ConsultStore(api: api, plant: .fixture(slug: "mona"))

        store.attach(jpeg: jpeg)
        store.composer = "here is the underside"
        await store.send()

        #expect(api.asked.first?.1.photo == jpeg)
        #expect(api.asked.first?.1.message == "here is the underside")
        #expect(store.attachment == nil, "a sent photo must not stay pending")
    }

    @Test("A photo on its own is a question, with no words at all")
    @MainActor
    func aPhotoAloneIsSendable() async {
        let api = FakeAPI()
        let store = ConsultStore(api: api, plant: nil, attachment: jpeg)

        #expect(store.canSend)
        await store.send()

        #expect(api.scratchAsks.count == 1)
        #expect(api.scratchAsks.first?.photo == jpeg)
        #expect(api.scratchAsks.first?.message == nil, "empty words are omitted, not sent blank")
    }

    @Test("Nothing typed and nothing attached is not a question")
    @MainActor
    func emptyIsNotSendable() async {
        let api = FakeAPI()
        let store = ConsultStore(api: api, plant: .fixture())

        store.composer = "   "
        #expect(!store.canSend)
        await store.send()

        #expect(api.asked.isEmpty)
        #expect(store.messages.isEmpty)
    }

    /// The photo is the one thing here that cannot be reconstructed, so a
    /// failure that drops it is unrecoverable in a way a lost sentence is not.
    @Test("A failed send keeps the photo")
    @MainActor
    func failureKeepsThePhoto() async {
        let api = FakeAPI()
        api.failure = .offline
        let store = ConsultStore(api: api, plant: .fixture())

        store.attach(jpeg: jpeg)
        store.composer = "is this scale?"
        await store.send()

        #expect(store.error == .offline)
        #expect(store.failed?.photo == jpeg)
        #expect(store.failed?.text == "is this scale?")
    }

    @Test("A failed scratch send keeps the photo too")
    @MainActor
    func scratchFailureKeepsThePhoto() async {
        let api = FakeAPI()
        api.failure = .timedOut
        let store = ConsultStore(api: api, plant: nil, attachment: jpeg)

        await store.send()

        #expect(store.error == .timedOut)
        #expect(store.failed?.photo == jpeg)
    }

    @Test("Retrying sends the same photo again, not just the words")
    @MainActor
    func retryResendsThePhoto() async {
        let api = FakeAPI()
        api.failure = .offline
        let store = ConsultStore(api: api, plant: .fixture(slug: "mona"))

        store.attach(jpeg: jpeg)
        store.composer = "look here"
        await store.send()

        api.failure = nil
        await store.retry()

        #expect(api.asked.count == 1, "the failed attempt never reached the fake")
        #expect(api.asked.first?.1.photo == jpeg)
        #expect(store.failed == nil)
        #expect(store.messages.count == 2, "the dangling question was left behind")
    }

    @Test("Editing a failed question hands the photo back, not only the words")
    @MainActor
    func recoveringADraftKeepsThePhoto() async {
        let api = FakeAPI()
        api.failure = .offline
        let store = ConsultStore(api: api, plant: .fixture())

        store.attach(jpeg: jpeg)
        store.composer = "what is this spot"
        await store.send()
        store.recoverDraft()

        #expect(store.composer == "what is this spot")
        #expect(store.attachment == jpeg)
        #expect(store.failed == nil)
        #expect(store.messages.isEmpty)
    }

    @Test("The transcript shows the photo next to the words it was sent with")
    @MainActor
    func transcriptCarriesThePhoto() async {
        let store = ConsultStore(api: FakeAPI(), plant: .fixture())

        store.attach(jpeg: jpeg)
        store.composer = "underside"
        await store.send()

        #expect(store.messages.first?.photo == jpeg)
        #expect(store.messages.last?.photo == nil, "the reply is not carrying our picture")
    }

    @Test("A second turn attaches a second photo without dragging the first along")
    @MainActor
    func attachmentsDoNotStack() async {
        let api = FakeAPI()
        let store = ConsultStore(api: api, plant: .fixture())

        store.attach(jpeg: jpeg)
        await store.send("first")
        store.attach(jpeg: otherJPEG)
        await store.send("second")

        #expect(api.asked.map(\.1.photo) == [jpeg, otherJPEG])
    }

    @Test("Removing an attachment actually removes it")
    @MainActor
    func attachmentIsRemovable() async {
        let api = FakeAPI()
        let store = ConsultStore(api: api, plant: .fixture())

        store.attach(jpeg: jpeg)
        store.removeAttachment()
        store.composer = "never mind"
        await store.send()

        #expect(store.attachment == nil)
        #expect(api.asked.first?.1.photo == nil)
    }
}

/// Photographing a stranger's fern must not add a pot to your library. This is
/// the property the whole scratch path exists for.
@Suite("Asking about a photo with no plant behind it")
struct ScratchConsultTests {
    @Test("A scratch chat creates no plant and touches no timeline")
    @MainActor
    func createsNothing() async {
        let api = FakeAPI()
        let store = ConsultStore(api: api, plant: nil, attachment: jpeg)

        store.composer = "what is this?"
        await store.send()
        await store.send("and is it safe for cats?")

        #expect(api.created.isEmpty, "a plant was created")
        #expect(api.uploads.isEmpty, "the photo was filed against a plant")
        #expect(api.observations.isEmpty, "something landed on a timeline")
        #expect(api.asked.isEmpty, "it asked a plant-scoped endpoint")
        #expect(api.scratchAsks.count == 2)
    }

    @Test("A scratch follow-up stays in the same conversation")
    @MainActor
    func followUpsStayInTheSameConversation() async {
        let api = FakeAPI()
        let store = ConsultStore(api: api, plant: nil)

        await store.send("what is this?")
        await store.send("is it safe for cats?")

        let first = api.scratchAsks.first?.conversationID
        #expect(first != nil)
        #expect(api.scratchAsks.last?.conversationID == first)
    }

    /// Same response shape as a plant consultation, so the follow-ups a chat
    /// with no plant behind it suggests are just as usable.
    @Test("The scratch answer lands in the transcript, follow-ups and all")
    @MainActor
    func answerIsShown() async {
        let api = FakeAPI()
        api.answer = .fixture(reply: "That is a peace lily.", followUps: ["Is it safe for cats?"])
        let store = ConsultStore(api: api, plant: nil)

        await store.send("what is this?")

        #expect(store.messages.count == 2)
        #expect(store.messages.last?.text == "That is a peace lily.")
        #expect(store.suggestedFollowUps == ["Is it safe for cats?"])
    }

    @Test("It says what it is without a plant to name")
    @MainActor
    func titleAndOpenersSuitTheScratchChat() {
        let scratch = ConsultStore(api: FakeAPI(), plant: nil)
        let consult = ConsultStore(api: FakeAPI(), plant: .fixture(commonName: "Mona"))

        #expect(scratch.title == "Ask Planty")
        #expect(consult.title == "Ask about Mona")
        #expect(scratch.openers != consult.openers)
    }

    /// The toxicity card sends people here with the question already asked;
    /// making them retype it would be the whole point of the button, undone.
    @Test("A seeded question is asked the moment the chat opens, once")
    @MainActor
    func aSeededQuestionAsksItself() async {
        let api = FakeAPI()
        let store = ConsultStore(api: api, plant: .fixture(slug: "mona"), pending: "is this dangerous?")

        await store.begin()
        await store.begin()

        #expect(api.asked.map(\.1.message) == ["is this dangerous?"])
    }
}
