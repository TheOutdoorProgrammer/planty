import Foundation
import Testing

@testable import Planty

@Suite("Notes on a plant")
struct NotesTests {
    @Test("Writing one keeps it, newest first")
    @MainActor
    func writingKeepsIt() async {
        let api = FakeAPI()
        let store = NotesStore(api: api, slug: "golden-pothos")

        let saved = await store.write(title: "The cat", body: "she keeps chewing this one")

        #expect(saved)
        #expect(store.notes.first?.body == "she keeps chewing this one")
        #expect(store.notes.first?.heading == "The cat")
    }

    @Test("An empty note is never written")
    @MainActor
    func emptyNotesAreNotWritten() async {
        let api = FakeAPI()
        let store = NotesStore(api: api, slug: "golden-pothos")

        let saved = await store.write(title: "Heading only", body: "   \n  ")

        #expect(!saved)
        #expect(store.notes.isEmpty)
    }

    /// A sheet closes on a true return, so a failure returning true would throw
    /// away whatever was typed into it.
    @Test("A failed write says so rather than claiming success")
    @MainActor
    func failedWriteIsReported() async {
        let api = FakeAPI()
        api.failure = .offline
        let store = NotesStore(api: api, slug: "golden-pothos")

        let saved = await store.write(title: "", body: "something worth keeping")

        #expect(!saved)
        #expect(store.error == .offline)
        #expect(store.notes.isEmpty)
    }

    @Test("Rewording the body keeps the heading")
    @MainActor
    func rewordingKeepsTheHeading() async {
        let api = FakeAPI()
        let store = NotesStore(api: api, slug: "golden-pothos")
        await store.write(title: "Repotting", body: "went up a size")
        guard let note = store.notes.first else {
            Issue.record("nothing was written")
            return
        }

        await store.rewrite(note, title: "Repotting", body: "went up two sizes")

        #expect(store.notes.first?.body == "went up two sizes")
        #expect(store.notes.first?.heading == "Repotting")
        #expect(store.notes.count == 1, "rewriting added a second note")
    }

    /// A note that vanishes locally and survives on the server comes back on
    /// the next load, which reads as the delete having been undone.
    @Test("A failed delete leaves the note where it is")
    @MainActor
    func failedDeleteKeepsTheNote() async {
        let api = FakeAPI()
        let store = NotesStore(api: api, slug: "golden-pothos")
        await store.write(title: "", body: "keep me")
        guard let note = store.notes.first else {
            Issue.record("nothing was written")
            return
        }
        api.failure = .offline

        await store.remove(note)

        #expect(store.notes.count == 1)
        #expect(store.error == .offline)
    }

    @Test("A successful delete removes it")
    @MainActor
    func deleteRemovesIt() async {
        let api = FakeAPI()
        let store = NotesStore(api: api, slug: "golden-pothos")
        await store.write(title: "", body: "temporary")
        guard let note = store.notes.first else {
            Issue.record("nothing was written")
            return
        }

        await store.remove(note)

        #expect(store.notes.isEmpty)
    }

    @Test("An untouched note does not claim to have been edited")
    func untouchedNotesAreNotEdited() {
        let note = PlantNote.fixture()
        #expect(!note.wasEdited)

        let changed = PlantNote.fixture(updatedAt: note.createdAt.addingTimeInterval(120))
        #expect(changed.wasEdited)
    }

    @Test("A blank heading is no heading")
    func blankHeadingsAreAbsent() {
        #expect(PlantNote.fixture(title: "  ").heading == nil)
        #expect(PlantNote.fixture(title: nil).heading == nil)
        #expect(PlantNote.fixture(title: "Real").heading == "Real")
    }
}

@Suite("What Planty did")
struct AnswerStepTests {
    /// Older answers have no steps at all, and the app has to keep working
    /// against a service that has not been updated yet.
    @Test("An answer with no steps still decodes")
    func stepsAreOptional() throws {
        let json = Data("""
        {"id":"\(UUID().uuidString)","conversation_id":"\(UUID().uuidString)",
         "reply":"hello","confidence":0.8,"looked_at":"","suggested_follow_ups":[]}
        """.utf8)

        let answer = try PlantyCoders.decoder().decode(PlantAnswer.self, from: json)
        #expect(answer.steps.isEmpty)
    }

    @Test("Steps decode with their tool and output")
    func stepsDecode() throws {
        let json = Data("""
        {"id":"\(UUID().uuidString)","conversation_id":"\(UUID().uuidString)",
         "reply":"hello","confidence":0.8,"looked_at":"","suggested_follow_ups":[],
         "steps":[{"kind":"action","tool":"WebFetch","detail":"https://www.aspca.org/x","output":"a page"},
                  {"kind":"thought","output":"thinking about it"}]}
        """.utf8)

        let answer = try PlantyCoders.decoder().decode(PlantAnswer.self, from: json)

        #expect(answer.steps.count == 2)
        #expect(answer.steps[0].headline == "Read a page")
        #expect(answer.steps[0].subtitle == "https://www.aspca.org/x")
        #expect(answer.steps[1].kind == .thought)
        #expect(answer.steps[1].headline == "Thought about it")
    }

    /// A kind the app has never heard of is still something that happened, and
    /// dropping the whole answer over it would be a worse outcome.
    @Test("An unknown kind reads as an action rather than failing")
    func unknownKindsSurvive() throws {
        let json = Data("""
        {"id":"\(UUID().uuidString)","conversation_id":"\(UUID().uuidString)",
         "reply":"hello","confidence":0.8,"looked_at":"","suggested_follow_ups":[],
         "steps":[{"kind":"telepathy","output":"who knows"}]}
        """.utf8)

        let answer = try PlantyCoders.decoder().decode(PlantAnswer.self, from: json)
        #expect(answer.steps.first?.kind == .action)
    }

    @Test("A refused command is marked as refused")
    func refusalsAreMarked() throws {
        let json = Data("""
        {"id":"\(UUID().uuidString)","conversation_id":"\(UUID().uuidString)",
         "reply":"hello","confidence":0.8,"looked_at":"","suggested_follow_ups":[],
         "steps":[{"kind":"action","tool":"Bash","detail":"rm -rf /","output":"failed: refused"}]}
        """.utf8)

        let answer = try PlantyCoders.decoder().decode(PlantAnswer.self, from: json)

        #expect(answer.steps.first?.refused == true)
        #expect(answer.steps.first?.icon == "exclamationmark.triangle")
    }
}
