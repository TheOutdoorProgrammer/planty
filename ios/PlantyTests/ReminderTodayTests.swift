import Foundation
import Testing

@testable import Planty

@MainActor
@Suite("Scheduled reminders on Today")
struct ReminderTodayTests {
    private func occurrence(
        reminderID: UUID = UUID(),
        dueAt: Date = .reference.minus(hours: 1),
        kind: ObservationKind = .misted
    ) -> DueReminder {
        let plant = Plant.fixture(slug: "oyster", commonName: "Blue oyster")
        return DueReminder(
            reminder: Reminder(
                id: reminderID,
                plantID: plant.id,
                kind: kind,
                everyDays: 1,
                atHours: [8, 20],
                active: true,
                note: "Mist the surface lightly",
                lastDone: nil,
                due: true
            ),
            plant: plant,
            lastDone: nil,
            dueAt: dueAt
        )
    }

    @Test("A due reminder is work, never an all-clear")
    func dueReminderIsAnAction() {
        let due = occurrence()
        let digest = Digest(
            date: .reference.minus(hours: 1),
            entries: [],
            dueReminders: [due],
            checked: 8
        )

        guard case .actions(let summary) = TodayPresentation.make(
            .fixture(digest: digest, knownPlantCount: 8)
        ) else {
            Issue.record("a due reminder presented as calm")
            return
        }
        #expect(summary.reminders == [due])
        #expect(summary.count == 1)
        #expect(summary.headline == "One thing. You've got this.")
        #expect(!digest.isAllClear)
    }

    @Test("Finishing a reminder logs its configured care and clears its card")
    func completionLogsTheReminderKind() async {
        let due = occurrence(kind: .misted)
        let api = FakeAPI()
        api.digest = Digest(
            date: .reference.minus(hours: 1),
            entries: [],
            dueReminders: [due],
            checked: 1
        )
        api.plantList = [due.plant]
        let store = TodayStore(api: api, isConfigured: true, clock: { .reference })

        await store.load()
        #expect(await store.complete(due) == nil)

        #expect(api.observations.first?.0 == due.plant.slug)
        #expect(api.observations.first?.1.kind == .misted)
        #expect(api.observations.first?.1.body == due.reminder.note)
        #expect(store.resolvedReminderOccurrenceIDs.contains(due.occurrenceID))
        guard case .calm = store.presentation else {
            Issue.record("the completed reminder remained on Today")
            return
        }
    }

    @Test("Two slots of one reminder are distinct occurrences")
    func slotsStayDistinct() {
        let reminderID = UUID()
        let morning = occurrence(reminderID: reminderID, dueAt: .reference.minus(hours: 1))
        let evening = occurrence(reminderID: reminderID, dueAt: .reference.minus(hours: 0.5))
        let digest = Digest(
            date: .reference.minus(hours: 1),
            entries: [],
            dueReminders: [morning, evening],
            checked: 1
        )

        #expect(morning.occurrenceID != evening.occurrenceID)
        let visible = digest.hiding(
            verdictIDs: [],
            reminderOccurrenceIDs: [morning.occurrenceID]
        )
        #expect(visible.dueReminders == [evening])
    }

    @Test("Missing an occurrence closes it without recording care")
    func missedDoesNotInventCare() async {
        let due = occurrence(kind: .fertilized)
        let api = FakeAPI()
        api.digest = Digest(
            date: .reference.minus(hours: 1),
            entries: [],
            dueReminders: [due],
            checked: 1
        )
        api.plantList = [due.plant]
        let store = TodayStore(api: api, isConfigured: true, clock: { .reference })

        await store.load()
        #expect(await store.resolve(due, as: .missed, note: "I was away") == nil)

        #expect(api.observations.isEmpty)
        #expect(api.reminderResolutions.count == 1)
        #expect(api.reminderResolutions.first?.2 == .missed)
        #expect(api.reminderResolutions.first?.3 == "I was away")
        #expect(store.resolvedReminderOccurrenceIDs.contains(due.occurrenceID))
        guard case .calm = store.presentation else {
            Issue.record("the missed occurrence remained due")
            return
        }
    }

    @Test("The confirmation says what will be recorded")
    func confirmationLabelsMatchHistory() {
        #expect(ObservationKind.misted.completionLabel == "I misted it")
        #expect(ObservationKind.watered.completionLabel == "I watered it")
        #expect(ObservationKind.fertilized.completionLabel == "I fed it")
    }
}
