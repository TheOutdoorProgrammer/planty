import SwiftUI

/// The chores nothing measures. A probe confirms watering; nothing confirms
/// misting, so without these the only thing tracking it is somebody's memory.
struct RemindersScreen: View {
    @State var store: RemindersStore
    @State private var editing: ReminderDraft?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                if store.reminders.isEmpty && !store.isLoading {
                    empty
                }
                ForEach(store.reminders) { reminder in
                    ReminderCard(
                        reminder: reminder,
                        done: { Task { await store.markDone(reminder) } },
                        toggle: { active in Task { await store.setActive(reminder, to: active) } },
                        edit: { editing = ReminderDraft(existing: reminder) },
                        remove: { Task { await store.delete(reminder) } }
                    )
                }
                if !store.addable.isEmpty {
                    addMenu
                }
                if let error = store.error {
                    StateMessage(
                        title: "Reminders could not be reached.",
                        message: error.errorDescription ?? "Try again in a moment.",
                        accent: PlantyColor.orange,
                        icon: "exclamationmark.triangle.fill"
                    ) {
                        Button("Dismiss") { store.clearError() }
                            .buttonStyle(SecondaryButtonStyle())
                    }
                }
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 16)
        }
        .plantyPage()
        .navigationTitle("Reminders")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await store.load() }
        .task { await store.load() }
        .sheet(item: $editing) { draft in
            ReminderSheet(draft: draft) { saved in
                Task { await store.save(saved) }
            }
        }
    }

    private var empty: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("Nothing is being tracked for \(store.plant.commonName).")
                .font(.headline)
                .foregroundStyle(PlantyColor.foreground)
            Text("""
                Sensors catch thirst. They cannot see a kit that needs misting \
                twice a day, or a plant that has not been fed since spring.
                """)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(padding: 16)
    }

    private var addMenu: some View {
        Menu {
            ForEach(store.addable, id: \.self) { kind in
                Button(kind.instruction) { editing = ReminderDraft(kind: kind) }
            }
        } label: {
            Label("Add a reminder", systemImage: "plus.circle.fill")
                .frame(maxWidth: .infinity)
        }
        .buttonStyle(SecondaryButtonStyle())
    }
}

struct ReminderCard: View {
    let reminder: Reminder
    let done: () -> Void
    let toggle: (Bool) -> Void
    let edit: () -> Void
    let remove: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Label(reminder.kind.instruction, systemImage: reminder.kind.symbol)
                    .font(.headline)
                    .foregroundStyle(PlantyColor.foreground)
                Spacer()
                if reminder.due == true && reminder.active {
                    Text("Due")
                        .font(.caption.weight(.bold))
                        .padding(.horizontal, 10)
                        .padding(.vertical, 4)
                        .background(PlantyColor.orange, in: Capsule())
                        .foregroundStyle(PlantyColor.background)
                }
            }

            Text(reminder.schedule)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)

            // Never done is worth saying plainly: it is exactly the plant a
            // reminder was set for, and "no history" reads as reassurance.
            Text(lastLine)
                .font(.caption)
                .foregroundStyle(PlantyColor.secondaryText)

            if let note = reminder.note, !note.isEmpty {
                Text(note)
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            HStack(spacing: 10) {
                Button("I did it", action: done)
                    .buttonStyle(PrimaryButtonStyle())
                Button(reminder.active ? "Pause" : "Resume") { toggle(!reminder.active) }
                    .buttonStyle(SecondaryButtonStyle())
            }

            Button("Remove", role: .destructive, action: remove)
                .font(.caption)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(padding: 16)
        .opacity(reminder.active ? 1 : 0.55)
        .contentShape(Rectangle())
        .onTapGesture(perform: edit)
    }

    private var lastLine: String {
        guard let lastDone = reminder.lastDone else { return "Never done" }
        let days = Calendar.current.dateComponents([.day], from: lastDone, to: Date()).day ?? 0
        if days <= 0 { return "Last done today" }
        return days == 1 ? "Last done yesterday" : "Last done \(days) days ago"
    }
}
