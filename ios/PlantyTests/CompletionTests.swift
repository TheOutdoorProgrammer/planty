import Foundation
import Testing

@testable import Planty

/// The two ways a card could go quiet without the plant being helped. Both
/// shipped, and both are the kind of bug that kills something slowly: the user
/// believes they did the thing, and the app agrees with them.
@Suite("Finishing a task")
struct CompletionTests {
    private func entry() -> DigestEntry {
        .fixture(action: .water)
    }

    /// "Note" used to sit among the completion options. One tap filed an empty
    /// note, acknowledged the verdict, and hid a thirsty plant.
    @Test("A note never counts as having done the job")
    @MainActor
    func notesDoNotComplete() async {
        let api = FakeAPI()
        let store = TodayStore(api: api, isConfigured: true)
        let card = entry()

        await store.addNote(card, text: "leaves look a bit limp")

        #expect(api.acknowledged.isEmpty, "writing a note silenced the card")
        #expect(!store.resolvedIDs.contains(card.verdict.id))
        #expect(api.observations.first?.1.kind == .note)
        #expect(api.observations.first?.1.body == "leaves look a bit limp",
                "an empty note tells the next diagnosis nothing")
    }

    @Test("An empty note is not written at all")
    @MainActor
    func emptyNotesAreNotWritten() async {
        let api = FakeAPI()
        let store = TodayStore(api: api, isConfigured: true)

        await store.addNote(entry(), text: "")

        #expect(api.observations.isEmpty)
    }

    @Test("Doing the job records it and settles the verdict")
    @MainActor
    func completingSettles() async {
        let api = FakeAPI()
        let store = TodayStore(api: api, isConfigured: true)
        let card = entry()

        await store.complete(card, kind: .watered, note: "topped it up")

        #expect(api.observations.first?.1.kind == .watered)
        #expect(api.observations.first?.1.body == "topped it up")
        #expect(api.acknowledged == [card.verdict.id])
        #expect(store.resolvedIDs.contains(card.verdict.id))
    }

    /// The acknowledgement is what stops the service escalating. Swallowing its
    /// failure hides the card locally while the server keeps chasing, which is
    /// the worst of both.
    @Test("A failed acknowledgement is admitted, not swallowed")
    @MainActor
    func failedAckIsReported() async {
        let api = FakeAPI()
        api.failAcknowledge = true
        let store = TodayStore(api: api, isConfigured: true)
        let card = entry()

        await store.complete(card, kind: .watered)

        #expect(store.actionError == .stillAsking)
        #expect(!store.resolvedIDs.contains(card.verdict.id),
                "the card was hidden while the service was still asking")
    }

    /// Tapping "Watered" and failing used to render "Today's check did not
    /// finish", which is a claim about the whole screen rather than the tap.
    @Test("A failed action does not read as a failed check")
    @MainActor
    func actionFailuresAreTheirOwnError() async {
        let api = FakeAPI()
        api.failure = .offline
        let store = TodayStore(api: api, isConfigured: true)

        await store.complete(entry(), kind: .watered)

        #expect(store.actionError == .offline)
        #expect(store.error == nil, "an action failure was reported as a load failure")
    }

    /// A capture started from a card and answering it must settle that card, or
    /// the user does everything right and the plant is asked for again.
    @Test("A capture that answers a card settles it")
    @MainActor
    func captureSettlesTheCardItAnswers() async {
        let api = FakeAPI()
        let capture = CaptureStore(api: api, selectedPlant: .fixture())
        let verdict = UUID()
        capture.answering = verdict

        capture.accept(jpeg: Data("photo".utf8))
        await capture.save(recording: .watered)

        #expect(api.acknowledged == [verdict], "the card was never acknowledged")
        #expect(capture.settled == verdict)
    }

    /// A photo with no claim attached settles nothing: looking at a plant is
    /// not watering it.
    @Test("A photo alone does not settle a card")
    @MainActor
    func aPhotoAloneSettlesNothing() async {
        let api = FakeAPI()
        let capture = CaptureStore(api: api, selectedPlant: .fixture())
        capture.answering = UUID()

        capture.accept(jpeg: Data("photo".utf8))
        await capture.save(recording: nil)

        #expect(api.acknowledged.isEmpty)
        #expect(capture.settled == nil)
    }
}
