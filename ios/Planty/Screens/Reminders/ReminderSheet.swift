import SwiftUI

/// A reminder being written. Separate from `Reminder` because a draft has no
/// id and its hours are being edited, not stored.
struct ReminderDraft: Identifiable, Sendable {
    let id = UUID()
    var kind: ObservationKind
    var everyDays: Int
    var hours: Set<Int>
    var active: Bool
    var note: String

    /// Every third hour. A full 24 is a wall of toggles, and nobody schedules
    /// a chore for 3am.
    static let standardHours = [6, 8, 10, 12, 14, 16, 18, 20, 22]

    init(kind: ObservationKind) {
        self.kind = kind
        // Misting is the twice-a-day case and the reason hours are a set, so
        // it opens already saying so rather than making you find it.
        self.everyDays = 1
        self.hours = kind == .misted ? [8, 20] : [8]
        self.active = true
        self.note = ""
    }

    init(existing: Reminder) {
        self.kind = existing.kind
        self.everyDays = existing.everyDays
        self.hours = Set(existing.atHours)
        // Dropping this is how editing a paused reminder used to resume it.
        self.active = existing.active
        self.note = existing.note ?? ""
    }

    var isValid: Bool { !hours.isEmpty && (1...365).contains(everyDays) }

    /// The standard grid plus whatever the reminder already holds. An hour set
    /// outside the grid (the chat agent's 07:00) must still render, or the
    /// sheet shows every toggle off while the hidden hour survives Save.
    var offeredHours: [Int] {
        Set(Self.standardHours).union(hours).sorted()
    }

    func asNew() -> NewReminder {
        NewReminder(
            kind: kind,
            everyDays: everyDays,
            atHours: hours.sorted(),
            active: active,
            note: note.isEmpty ? nil : note
        )
    }
}

struct ReminderSheet: View {
    @State var draft: ReminderDraft
    let save: (NewReminder) async -> PlantyError?

    @Environment(\.dismiss) private var dismiss

    /// Frozen at open: unticking an off-grid hour must not delete its row, or
    /// there would be no way to tick it back before saving.
    private let offeredHours: [Int]

    @State private var action = AsyncSheetAction()

    init(draft: ReminderDraft, save: @escaping (NewReminder) async -> PlantyError?) {
        _draft = State(initialValue: draft)
        self.save = save
        offeredHours = draft.offeredHours
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("How often") {
                    Stepper(
                        everyDaysLabel,
                        value: $draft.everyDays,
                        in: 1...90
                    )
                }

                Section("At") {
                    ForEach(offeredHours, id: \.self) { hour in
                        Toggle(Reminder.hourLabel(hour), isOn: binding(for: hour))
                    }
                    if draft.hours.isEmpty {
                        Text("Pick at least one time, or nothing will ever fire.")
                            .font(.caption)
                            .foregroundStyle(PlantyColor.orange)
                    }
                }

                Section("Note") {
                    TextField("Anything worth remembering", text: $draft.note, axis: .vertical)
                        .lineLimit(1...3)
                }

                Section {
                    Toggle(draft.active ? "Active" : "Paused", isOn: $draft.active)
                } footer: {
                    Text("Paused reminders keep their schedule but never come due.")
                }

                if let failure = action.error {
                    Section {
                        Label {
                            Text(failureText(failure))
                        } icon: {
                            Image(systemName: "exclamationmark.triangle.fill")
                        }
                        .foregroundStyle(PlantyColor.orange)
                    }
                }
            }
            .navigationTitle(draft.kind.instruction)
            .navigationBarTitleDisplayMode(.inline)
            .interactiveDismissDisabled(action.isRunning)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .disabled(action.isRunning)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(action.isRunning ? "Saving…" : "Save") {
                        Task { await attemptSave() }
                    }
                    .disabled(!draft.isValid || action.isRunning)
                }
            }
        }
    }

    /// Dismisses only once the service has said yes. A failure keeps the sheet
    /// and the draft on screen instead of throwing the input away.
    private func attemptSave() async {
        guard await action.perform({ await save(draft.asNew()) }) else { return }
        dismiss()
    }

    private func failureText(_ failure: PlantyError) -> String {
        let reason = failure.errorDescription ?? "The service did not answer."
        return "\(reason) Nothing was saved; your changes are still here."
    }

    private var everyDaysLabel: String {
        draft.everyDays == 1 ? "Every day" : "Every \(draft.everyDays) days"
    }

    private func binding(for hour: Int) -> Binding<Bool> {
        Binding(
            get: { draft.hours.contains(hour) },
            set: { isOn in
                if isOn {
                    draft.hours.insert(hour)
                } else {
                    draft.hours.remove(hour)
                }
            }
        )
    }
}
