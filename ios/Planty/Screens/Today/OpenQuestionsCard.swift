import SwiftUI

/// What Planty wants settled that only a person can settle. Mostly for the
/// friend whose plants these are, which is the case the queue exists for: they
/// are not in the conversation to be asked.
struct OpenQuestionsCard: View {
    let questions: [OpenQuestion]
    let answer: (OpenQuestion, String) async -> PlantyError?

    @State private var answering: OpenQuestion?

    var body: some View {
        if !questions.isEmpty {
            VStack(alignment: .leading, spacing: 12) {
                Eyebrow(text: "Waiting on a person", color: PlantyColor.cyan)

                Text(headline)
                    .font(.headline)
                    .foregroundStyle(PlantyColor.foreground)

                ForEach(questions.prefix(4)) { question in
                    row(question)
                }

                if questions.count > 4 {
                    Text("and \(questions.count - 4) more")
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .plantyCard(border: PlantyColor.cyan.opacity(0.35))
            .sheet(item: $answering) { question in
                AnswerSheet(question: question) { text in
                    await answer(question, text)
                }
            }
        }
    }

    private var headline: String {
        let names = Set(questions.filter(\.isForSomebodyElse).map(\.askedOf))
        guard let only = names.first, names.count == 1 else {
            return questions.count == 1
                ? "One thing Planty wants to know"
                : "\(questions.count) things Planty wants to know"
        }
        return questions.count == 1
            ? "One thing to ask \(only)"
            : "\(questions.count) things to ask \(only)"
    }

    private func row(_ question: OpenQuestion) -> some View {
        Button {
            answering = question
        } label: {
            VStack(alignment: .leading, spacing: 3) {
                Text(question.question)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.foreground)
                    .multilineTextAlignment(.leading)
                if let why = question.why, !why.isEmpty {
                    Text(why)
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                        .multilineTextAlignment(.leading)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .frame(minHeight: 44)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityHint("Record the answer")
    }
}

/// Records what the person said, in their words rather than a summary.
struct AnswerSheet: View {
    let question: OpenQuestion
    let save: (String) async -> PlantyError?

    @Environment(\.dismiss) private var dismiss
    @State private var text = ""
    @State private var saving = false
    @State private var failure: PlantyError?

    var body: some View {
        NavigationStack {
            Form {
                if let failure {
                    Section {
                        SheetErrorRow(
                            headline: "Not saved. Your answer is still here.",
                            error: failure
                        )
                    }
                }
                Section {
                    Text(question.question)
                        .font(.headline)
                        .foregroundStyle(PlantyColor.foreground)
                    if let why = question.why, !why.isEmpty {
                        Text(why)
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                }
                Section("What they said") {
                    TextField("In their words", text: $text, axis: .vertical)
                        .lineLimit(3...10)
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Answer")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .disabled(saving)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        saving = true
                        failure = nil
                        Task {
                            failure = await save(text)
                            saving = false
                            if failure == nil { dismiss() }
                        }
                    }
                    .disabled(text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || saving)
                }
            }
        }
        .interactiveDismissDisabled(saving)
    }
}
