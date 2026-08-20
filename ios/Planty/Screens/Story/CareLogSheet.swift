import SwiftUI

/// Recording care by hand, without the chat and without a photograph. The
/// note field is first-class: a symptom nobody describes cannot be judged.
struct CareLogSheet: View {
    let plantName: String
    let record: (ObservationKind, String?) async -> PlantyError?

    @Environment(\.dismiss) private var dismiss
    @State private var kind = ObservationKind.watered
    @State private var note = ""
    @State private var action = AsyncSheetAction()

    private static let kinds: [ObservationKind] = [
        .watered, .misted, .fertilized, .pruned, .repotted, .moved, .note, .symptom
    ]

    var body: some View {
        NavigationStack {
            Form {
                if let error = action.error {
                    Section {
                        SheetErrorRow(
                            headline: "Not recorded. Your note is still here.",
                            error: error
                        )
                    }
                }
                Section("What happened") {
                    Picker("What happened", selection: $kind) {
                        ForEach(Self.kinds, id: \.self) {
                            Label($0.label, systemImage: $0.symbol).tag($0)
                        }
                    }
                    .pickerStyle(.inline)
                    .labelsHidden()
                }
                Section {
                    TextField(notePrompt, text: $note, axis: .vertical)
                        .lineLimit(3...6)
                        .accessibilityLabel("Note")
                } footer: {
                    Text(noteFooter)
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Log care for \(plantName)")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .disabled(action.isRunning)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button {
                        Task { await submit() }
                    } label: {
                        if action.isRunning {
                            ProgressView()
                        } else {
                            Text("Save")
                        }
                    }
                    .disabled(action.isRunning || needsWords)
                    .accessibilityLabel(action.isRunning ? "Saving" : "Save this entry")
                }
            }
        }
        .interactiveDismissDisabled(action.isRunning)
    }

    /// A bare "note" or "symptom" entry says nothing; those two require words.
    private var needsWords: Bool {
        (kind == .note || kind == .symptom) && note.cleaned.isEmpty
    }

    private var notePrompt: String {
        switch kind {
        case .symptom: "What are you seeing?"
        case .note: "What is worth remembering?"
        case .moved: "Where did it go? (optional)"
        default: "Add a note (optional)"
        }
    }

    private var noteFooter: String {
        switch kind {
        case .symptom: "Say which leaves, what colour, and since when. Planty reads this."
        case .note: "This lands in \(plantName)'s story, word for word."
        default: "Optional. It helps Planty judge the next photo."
        }
    }

    private func submit() async {
        let trimmed = note.cleaned
        guard await action.perform({
            await record(kind, trimmed.isEmpty ? nil : trimmed)
        }) else { return }
        dismiss()
    }
}
