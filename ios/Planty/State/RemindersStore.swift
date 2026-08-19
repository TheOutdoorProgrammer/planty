import Foundation
import Observation

/// The reminders set on one plant, and what is owed right now. Whether a
/// reminder is due is computed by the service from real observations, never
/// here: a notification nobody acted on has to stay due.
@Observable
@MainActor
final class RemindersStore {
    private(set) var reminders: [Reminder] = []
    private(set) var isLoading = false
    private(set) var hasLoaded = false
    private(set) var error: PlantyError?

    let plant: Plant
    private let api: any PlantyAPI

    init(api: any PlantyAPI, plant: Plant) {
        self.api = api
        self.plant = plant
    }

    /// Kinds with no reminder yet, so the picker never offers a duplicate:
    /// one schedule per kind per plant is the service's rule too.
    var addable: [ObservationKind] {
        let taken = Set(reminders.map(\.kind))
        return Reminder.remindable.filter { !taken.contains($0) }
    }

    func load() async {
        isLoading = true
        defer { isLoading = false }

        do {
            reminders = try await api.reminders(slug: plant.slug)
            error = nil
            hasLoaded = true
        } catch {
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
            hasLoaded = true
        }
    }

    /// Returns the failure instead of banking it, so the sheet that owns the
    /// draft can keep the draft on screen. Callers with no sheet of their own
    /// bank the failure into `error` themselves.
    func save(_ reminder: NewReminder) async -> PlantyError? {
        do {
            let saved = try await api.setReminder(slug: plant.slug, reminder: reminder)
            if let index = reminders.firstIndex(where: { $0.kind == saved.kind }) {
                reminders[index] = saved
            } else {
                reminders.append(saved)
            }
            // Due-ness comes from the service, and a fresh reminder has none.
            await load()
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    func setActive(_ reminder: Reminder, to active: Bool) async {
        let failure = await save(
            NewReminder(
                kind: reminder.kind,
                everyDays: reminder.everyDays,
                atHours: reminder.atHours,
                active: active,
                note: reminder.note
            )
        )
        if let failure { error = failure }
    }

    func delete(_ reminder: Reminder) async {
        do {
            try await api.deleteReminder(slug: plant.slug, kind: reminder.kind)
            reminders.removeAll { $0.kind == reminder.kind }
        } catch {
            self.error = PlantyError.from(error)
        }
    }

    /// Records that the chore was done, which is the only thing that moves a
    /// reminder's next due date.
    func markDone(_ reminder: Reminder) async {
        // A paused reminder must not record: one mis-tap would forge a watering
        // claim, and the service's sensor verification then reads it as a clog.
        guard reminder.active else { return }
        do {
            _ = try await api.addObservation(
                slug: plant.slug,
                observation: NewObservation(kind: reminder.kind)
            )
            await load()
        } catch {
            self.error = PlantyError.from(error)
        }
    }

    func clearError() { error = nil }
}
