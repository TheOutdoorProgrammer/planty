import SwiftUI

struct QuestionsScreen: View {
    @Bindable var store: GardenStore
    let plants: [Plant]

    @State private var creating = false
    @State private var answering: OpenQuestion?

    var body: some View {
        List {
            Section {
                Picker("Question status", selection: $store.questionStatus) {
                    ForEach(QuestionStatus.allCases, id: \.self) { status in
                        Text(status.label).tag(status)
                    }
                }
                .pickerStyle(.segmented)
                .listRowBackground(PlantyColor.surface)
            }

            if store.questions.isEmpty {
                ContentUnavailableView(
                    store.questionStatus.emptyTitle,
                    systemImage: "checkmark.bubble.fill",
                    description: Text(store.questionStatus.emptyMessage)
                )
                .foregroundStyle(PlantyColor.secondaryText)
                .listRowBackground(Color.clear)
            } else {
                Section("\(store.questions.count) \(store.questionStatus.label.lowercased())") {
                    ForEach(store.questions) { question in
                        Button {
                            if store.questionStatus == .open { answering = question }
                        } label: {
                            QuestionRow(question: question)
                        }
                        .buttonStyle(.plain)
                        .disabled(store.questionStatus != .open)
                        .listRowBackground(PlantyColor.surface)
                    }
                }
            }
        }
        .plantyPage()
        .navigationTitle("Questions")
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button { creating = true } label: { Image(systemName: "plus") }
                    .accessibilityLabel("Add a question")
            }
        }
        .onChange(of: store.questionStatus) { _, _ in
            Task { await store.loadQuestions() }
        }
        .refreshable { await store.loadQuestions() }
        .sheet(item: $answering) { question in
            AnswerSheet(question: question) { answer in
                await store.answer(question, with: answer)
            }
        }
        .sheet(isPresented: $creating) {
            NewQuestionSheet(plants: plants) { draft in
                await store.createQuestion(draft)
            }
        }
    }
}

private struct QuestionRow: View {
    let question: OpenQuestion

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(question.question)
                .font(.headline)
                .foregroundStyle(PlantyColor.foreground)
                .multilineTextAlignment(.leading)
            Label("Asked of \(question.audience)", systemImage: "person.fill")
                .font(.caption)
                .foregroundStyle(PlantyColor.cyan)
            if let why = question.why, !why.isEmpty {
                Text(why)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
                    .multilineTextAlignment(.leading)
            }
            if let answer = question.answer, !answer.isEmpty {
                Text(answer)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.green)
                    .padding(.top, 2)
            }
        }
        .padding(.vertical, 6)
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

private struct NewQuestionSheet: View {
    let plants: [Plant]
    let save: (NewOpenQuestion) async -> PlantyError?

    @Environment(\.dismiss) private var dismiss
    @State private var question = ""
    @State private var askedOf = ""
    @State private var why = ""
    @State private var plantID: UUID?
    @State private var saving = false
    @State private var failure: PlantyError?

    var body: some View {
        NavigationStack {
            Form {
                if let failure {
                    Section { SheetErrorRow(headline: "Question not saved.", error: failure) }
                }
                Section("The question") {
                    TextField("What needs an answer?", text: $question, axis: .vertical)
                        .lineLimit(2...6)
                    TextField("Why it matters (optional)", text: $why, axis: .vertical)
                        .lineLimit(2...5)
                }
                Section("Who and what") {
                    TextField("Who can answer?", text: $askedOf)
                    Picker("Plant", selection: $plantID) {
                        Text("Whole garden").tag(UUID?.none)
                        ForEach(plants) { plant in
                            Text(plant.commonName).tag(Optional(plant.id))
                        }
                    }
                }
            }
            .plantyPage()
            .navigationTitle("New question")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }.disabled(saving)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await submit() } }
                        .disabled(question.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || saving)
                }
            }
        }
        .interactiveDismissDisabled(saving)
    }

    private func submit() async {
        saving = true
        failure = await save(
            NewOpenQuestion(
                plantID: plantID,
                askedOf: askedOf.nilIfBlank,
                question: question.trimmingCharacters(in: .whitespacesAndNewlines),
                why: why.nilIfBlank
            )
        )
        saving = false
        if failure == nil { dismiss() }
    }
}

private extension QuestionStatus {
    var label: String {
        switch self {
        case .open: "Open"
        case .answered: "Answered"
        case .dropped: "Dropped"
        }
    }

    var emptyTitle: String {
        switch self {
        case .open: "Nothing waiting"
        case .answered: "No answers yet"
        case .dropped: "Nothing dropped"
        }
    }

    var emptyMessage: String {
        switch self {
        case .open: "Every question has an answer."
        case .answered: "Answered questions will collect here."
        case .dropped: "Questions you stop pursuing will collect here."
        }
    }
}
