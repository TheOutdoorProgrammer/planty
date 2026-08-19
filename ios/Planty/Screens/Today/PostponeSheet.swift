import SwiftUI

/// "Not now" postpones for an explicit interval. It never quietly marks the
/// task complete, which is a different claim entirely.
struct PostponeSheet: View {
    let entry: DigestEntry
    let postpone: (PostponeInterval) -> Void
    let handled: () -> Void

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 14) {
                Text("\(entry.plant.commonName) can wait a bit.")
                    .font(.title3.weight(.bold))
                Text("Planty will bring this back. Nothing is marked done.")
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)

                ForEach(PostponeInterval.allCases) { interval in
                    Button(interval.label) { postpone(interval) }
                        .buttonStyle(SecondaryButtonStyle())
                }

                Button("I already handled it", action: handled)
                    .buttonStyle(PrimaryButtonStyle(color: PlantyColor.green))

                Spacer(minLength: 0)
            }
            .padding(20)
            .frame(maxWidth: .infinity, alignment: .leading)
            .plantyPage()
            .navigationTitle("Not now")
            .navigationBarTitleDisplayMode(.inline)
        }
        .presentationDetents([.medium])
    }
}

/// What happened, and whether that finishes the job. Only doing the thing may
/// silence a card: "Note" used to sit in this list, and one tap on a water card
/// filed an empty note, acknowledged the verdict and hid a thirsty plant.
struct HandledSheet: View {
    let entry: DigestEntry
    let record: (ObservationKind, String) -> Void
    let noteOnly: (String) -> Void

    @State private var detail = ""
    @State private var writing: ObservationKind?
    @FocusState private var detailFocused: Bool

    /// Only what genuinely completes this verdict. A note never does.
    private var completions: [ObservationKind] {
        switch entry.verdict.action {
        case .water: [.watered]
        case .harvest: [.harvested]
        case .check, .urgent: [.symptom, .moved, .watered]
        case .none, .unknown: []
        }
    }

    /// Kinds worth describing. "Symptom noted" with no words is a record that
    /// tells the next diagnosis nothing at all.
    private func wantsWords(_ kind: ObservationKind) -> Bool {
        kind == .symptom || kind == .note
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 14) {
                    if let writing {
                        detailEntry(for: writing)
                    } else {
                        chooser
                    }
                }
                .padding(20)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .plantyPage()
            .navigationTitle(entry.plant.commonName)
            .navigationBarTitleDisplayMode(.inline)
        }
        .presentationDetents([.medium, .large])
    }

    private var chooser: some View {
        VStack(alignment: .leading, spacing: 14) {
            Text("What did you do?")
                .font(.title3.weight(.bold))
            Text("This marks the task done. Planty will stop asking.")
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)

            ForEach(completions, id: \.self) { kind in
                Button {
                    if wantsWords(kind) {
                        writing = kind
                        detailFocused = true
                    } else {
                        record(kind, "")
                    }
                } label: {
                    Label(kind.label, systemImage: kind.symbol)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .buttonStyle(SecondaryButtonStyle())
            }

            Divider().overlay(PlantyColor.quietDecoration)

            // Deliberately below the line and worded as not-done: writing
            // something down leaves the task exactly where it was.
            Text("Not done yet?")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(PlantyColor.secondaryText)
            Button {
                writing = .note
                detailFocused = true
            } label: {
                Label("Just leave a note", systemImage: "text.alignleft")
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .buttonStyle(SecondaryButtonStyle())
            Text("The card stays until the job is actually done.")
                .font(.caption)
                .foregroundStyle(PlantyColor.secondaryText)
        }
    }

    private func detailEntry(for kind: ObservationKind) -> some View {
        VStack(alignment: .leading, spacing: 14) {
            Text(kind == .note ? "What is worth remembering?" : "What did you see?")
                .font(.title3.weight(.bold))

            TextField("A sentence is plenty", text: $detail, axis: .vertical)
                .lineLimit(3...6)
                .focused($detailFocused)
                .padding(12)
                .background(PlantyColor.surface, in: RoundedRectangle(cornerRadius: 14))

            Button(kind == .note ? "Save the note" : "Record it") {
                let text = detail.trimmingCharacters(in: .whitespacesAndNewlines)
                if kind == .note {
                    noteOnly(text)
                } else {
                    record(kind, text)
                }
            }
            .buttonStyle(PrimaryButtonStyle(color: PlantyColor.green))
            .disabled(detail.trimmingCharacters(in: .whitespaces).isEmpty)

            Button("Back") {
                writing = nil
                detail = ""
            }
            .buttonStyle(SecondaryButtonStyle())

            if kind == .note {
                Text("This does not mark anything done.")
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
    }
}
