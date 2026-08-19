import SwiftUI

/// A reminder being written. Separate from `Reminder` because a draft has no
/// id and its hours are being edited, not stored.
struct ReminderDraft: Identifiable, Sendable {
    let id = UUID()
    var kind: ObservationKind
    var everyDays: Int
    var hours: Set<Int>
    var note: String

    init(kind: ObservationKind) {
        self.kind = kind
        // Misting is the twice-a-day case and the reason hours are a set, so
        // it opens already saying so rather than making you find it.
        self.everyDays = 1
        self.hours = kind == .misted ? [8, 20] : [8]
        self.note = ""
    }

    init(existing: Reminder) {
        self.kind = existing.kind
        self.everyDays = existing.everyDays
        self.hours = Set(existing.atHours)
        self.note = existing.note ?? ""
    }

    var isValid: Bool { !hours.isEmpty && (1...365).contains(everyDays) }

    func asNew(active: Bool = true) -> NewReminder {
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
    let save: (NewReminder) -> Void

    @Environment(\.dismiss) private var dismiss

    /// Every third hour. A full 24 is a wall of toggles, and nobody schedules
    /// a chore for 3am.
    private let choosableHours = [6, 8, 10, 12, 14, 16, 18, 20, 22]

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
                    ForEach(choosableHours, id: \.self) { hour in
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
            }
            .navigationTitle(draft.kind.instruction)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        save(draft.asNew())
                        dismiss()
                    }
                    .disabled(!draft.isValid)
                }
            }
        }
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
