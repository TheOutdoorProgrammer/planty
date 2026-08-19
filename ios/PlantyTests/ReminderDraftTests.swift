import Foundation
import Testing

@testable import Planty

/// The draft is the only thing standing between the sheet and a wire write,
/// so the two lies it used to tell — resuming a paused reminder and hiding an
/// off-grid hour — are pinned here.
@Suite("Reminder draft")
struct ReminderDraftTests {
    @Test("Editing a paused reminder keeps it paused")
    func carriesPauseThroughEditing() {
        let draft = ReminderDraft(existing: .fixture(kind: .watered, active: false))
        #expect(!draft.active)
        #expect(!draft.asNew().active)
    }

    @Test("A fresh reminder starts active")
    func startsActive() {
        let draft = ReminderDraft(kind: .watered)
        #expect(draft.active)
        #expect(draft.asNew().active)
    }

    @Test("An hour outside the grid is still offered")
    func offersOffGridHours() {
        let draft = ReminderDraft(existing: .fixture(kind: .watered, atHours: [7]))
        #expect(draft.offeredHours.contains(7))
        #expect(draft.offeredHours == draft.offeredHours.sorted())
        // The grid itself survives alongside the stray hour.
        for hour in ReminderDraft.standardHours {
            #expect(draft.offeredHours.contains(hour))
        }
    }

    @Test("On-grid hours add nothing to the grid")
    func gridOnlyWhenNothingStrays() {
        let draft = ReminderDraft(existing: .fixture(kind: .misted, atHours: [8, 20]))
        #expect(draft.offeredHours == ReminderDraft.standardHours)
    }

    @Test("Misting opens as the twice-a-day case")
    func mistingDefaultsToTwiceDaily() {
        #expect(ReminderDraft(kind: .misted).hours == [8, 20])
        #expect(ReminderDraft(kind: .watered).hours == [8])
    }

    @Test("No hours means nothing would ever fire")
    func requiresAtLeastOneHour() {
        var draft = ReminderDraft(kind: .watered)
        draft.hours = []
        #expect(!draft.isValid)
    }

    @Test("The wire form sorts hours and drops an empty note")
    func buildsTheWireForm() {
        var draft = ReminderDraft(kind: .fertilized)
        draft.hours = [20, 8]
        draft.everyDays = 14
        draft.note = ""

        let new = draft.asNew()
        #expect(new.atHours == [8, 20])
        #expect(new.everyDays == 14)
        #expect(new.note == nil)
        #expect(new.kind == .fertilized)
    }
}
