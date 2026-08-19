import SwiftUI

/// Ask about a plant without photographing it first. The record is the subject,
/// and any pictures are offered to the model rather than attached, so a simple
/// question stays cheap and quick.
struct ConsultScreen: View {
    @State var store: ConsultStore
    @FocusState private var composerFocused: Bool

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    if !store.hasStarted {
                        opening
                    }
                    ForEach(store.messages) { message in
                        row(message)
                            .id(message.id)
                    }
                    if store.isThinking {
                        ThinkingRow(stageLine: "Reading \(store.plant.commonName)'s record.")
                    }
                    if let error = store.error {
                        askError(error)
                    }
                    followUps
                }
                .padding(.horizontal, 20)
                .padding(.vertical, 16)
            }
            .onChange(of: store.messages.count) { _, _ in
                guard let last = store.messages.last else { return }
                withAnimation { proxy.scrollTo(last.id, anchor: .bottom) }
            }
        }
        .plantyPage()
        .navigationTitle("Ask about \(store.plant.commonName)")
        .navigationBarTitleDisplayMode(.inline)
        .safeAreaInset(edge: .bottom) { composer }
    }

    /// An empty chat is a blank stare, so it opens with what it already knows
    /// and three questions its own record can answer.
    private var opening: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text("Ask me anything about \(store.plant.commonName).")
                .font(.headline)
                .foregroundStyle(PlantyColor.foreground)

            Text("""
                I have its watering log, its readings and what earlier photos \
                showed. I will only open a photo if seeing one would change \
                the answer.
                """)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)

            ForEach(store.openers, id: \.self) { prompt in
                Button(prompt) {
                    composerFocused = false
                    Task { await store.send(prompt) }
                }
                .buttonStyle(SecondaryButtonStyle())
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(padding: 16)
    }

    @ViewBuilder
    private func row(_ message: ConsultMessage) -> some View {
        switch message.speaker {
        case .user:
            HStack {
                Spacer(minLength: 40)
                Text(message.text)
                    .font(.body)
                    .foregroundStyle(PlantyColor.background)
                    .padding(.horizontal, 14)
                    .padding(.vertical, 10)
                    .background(PlantyColor.pink, in: RoundedRectangle(cornerRadius: 18))
            }
        case .planty:
            answerCard(message)
        }
    }

    private func answerCard(_ message: ConsultMessage) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(message.text)
                .font(.body)
                .foregroundStyle(PlantyColor.foreground)
                .frame(maxWidth: .infinity, alignment: .leading)

            if let answer = message.answer, answer.didOpenAPhotograph {
                Label(answer.lookedAt ?? "", systemImage: "eye")
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .plantyCard(padding: 14)
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private var followUps: some View {
        if !store.suggestedFollowUps.isEmpty, !store.isThinking {
            VStack(alignment: .leading, spacing: 8) {
                ForEach(store.suggestedFollowUps, id: \.self) { prompt in
                    Button(prompt) {
                        composerFocused = false
                        Task { await store.send(prompt) }
                    }
                    .buttonStyle(SecondaryButtonStyle())
                }
            }
        }
    }

    /// Offers the question back rather than only an apology: retyping what you
    /// just said is the worst possible response to a failure.
    private func askError(_ error: PlantyError) -> some View {
        StateMessage(
            title: "Planty could not answer that.",
            message: error.errorDescription ?? "Try again in a moment.",
            accent: PlantyColor.orange,
            icon: "exclamationmark.triangle.fill"
        ) {
            VStack(spacing: 8) {
                if store.failed != nil {
                    Button("Ask again") {
                        Task { await store.retry() }
                    }
                    .buttonStyle(PrimaryButtonStyle())

                    Button("Edit the question") { store.recoverDraft() }
                        .buttonStyle(SecondaryButtonStyle())
                } else {
                    Button("Dismiss") { store.clearError() }
                        .buttonStyle(SecondaryButtonStyle())
                }
            }
        }
    }

    private var composer: some View {
        HStack(spacing: 10) {
            TextField("Ask about this plant…", text: $store.composer, axis: .vertical)
                .textFieldStyle(.plain)
                .lineLimit(1...4)
                .focused($composerFocused)
                .padding(12)
                .background(PlantyColor.surface, in: Capsule())

            Button {
                composerFocused = false
                Task { await store.send() }
            } label: {
                Image(systemName: "arrow.up.circle.fill")
                    .font(.title)
                    .foregroundStyle(PlantyColor.pink)
            }
            .frame(minWidth: 44, minHeight: 44)
            .accessibilityLabel("Send question")
            .disabled(store.composer.trimmingCharacters(in: .whitespaces).isEmpty || store.isThinking)
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 10)
        .background(.ultraThinMaterial)
    }
}
