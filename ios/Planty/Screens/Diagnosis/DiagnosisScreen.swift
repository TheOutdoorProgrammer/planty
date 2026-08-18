import SwiftUI

/// Begins with a photo and a plant, never an empty composer. The reply keeps
/// observation, interpretation and today's action visibly apart.
struct DiagnosisScreen: View {
    @State var store: DiagnosisStore
    @FocusState private var composerFocused: Bool

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    if let photo = store.photo {
                        capturedPhoto(photo)
                    }
                    ForEach(store.messages) { message in
                        MessageRow(message: message, plant: store.plant) { action in
                            Task { await store.perform(action) }
                        }
                        .id(message.id)
                    }
                    if store.isThinking {
                        ThinkingRow(stageLine: store.stageLine)
                    }
                    if let error = store.error {
                        DiagnosisErrorCard(error: error, plantName: store.plant.commonName) {
                            Task { await store.send(followUp: "Try that comparison again") }
                        }
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
        .navigationTitle(store.plant.commonName)
        .navigationBarTitleDisplayMode(.inline)
        .safeAreaInset(edge: .bottom) { composer }
        .task { await store.begin(comparingAgainst: 0) }
    }

    private func capturedPhoto(_ photo: CapturedPhoto) -> some View {
        Group {
            if let image = UIImage(data: photo.jpeg) {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFit()
                    .clipShape(RoundedRectangle(cornerRadius: 20, style: .continuous))
            }
        }
        .accessibilityLabel("The photo this conversation is about")
    }

    @ViewBuilder
    private var followUps: some View {
        if !store.suggestedFollowUps.isEmpty, !store.isThinking {
            VStack(alignment: .leading, spacing: 8) {
                ForEach(store.suggestedFollowUps, id: \.self) { prompt in
                    Button(prompt) {
                        Task { await store.send(followUp: prompt) }
                    }
                    .buttonStyle(SecondaryButtonStyle())
                }
            }
        }
    }

    private var composer: some View {
        HStack(spacing: 10) {
            TextField("Ask a follow-up…", text: $store.composer, axis: .vertical)
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
            .accessibilityLabel("Send follow-up")
            .disabled(store.composer.trimmingCharacters(in: .whitespaces).isEmpty)
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 10)
        .background(.ultraThinMaterial)
    }
}

/// Progress is only claimed when it is true; no rotating fake status phrases.
struct ThinkingRow: View {
    let stageLine: String?

    var body: some View {
        HStack(spacing: 10) {
            ProgressView().tint(PlantyColor.cyan)
            VStack(alignment: .leading, spacing: 2) {
                Text("Looking closer…")
                    .font(.subheadline.weight(.semibold))
                if let stageLine {
                    Text(stageLine)
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(padding: 14)
        .accessibilityElement(children: .combine)
    }
}

struct DiagnosisErrorCard: View {
    let error: PlantyError
    let plantName: String
    let retry: () -> Void

    var body: some View {
        if case .offline = error {
            StateMessage(
                title: "Photo saved. Diagnosis is waiting for a connection.",
                message: "You can close Planty. The answer will appear here when it is ready.",
                accent: PlantyColor.cyan,
                icon: "wifi.slash"
            ) { EmptyView() }
        } else {
            StateMessage(
                title: "Planty could not finish this comparison.",
                message: """
                    The photo is safely in \(plantName)'s story. Try the \
                    diagnosis again, or add a note about what you saw.
                    """,
                accent: PlantyColor.orange,
                icon: "exclamationmark.bubble"
            ) {
                Button("Try again", action: retry)
                    .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
            }
        }
    }
}
