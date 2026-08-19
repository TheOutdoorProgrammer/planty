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
                        MessageRow(message: message, plant: store.plant, taken: store.taken) { action in
                            Task { await store.perform(action) }
                        }
                        .id(message.id)
                    }
                    if store.isThinking {
                        ThinkingRow(stageLine: store.stageLine)
                    }
                    if let error = store.error {
                        DiagnosisErrorCard(
                            error: error,
                            plantName: store.plant.commonName,
                            photoIsSaved: store.photo?.uploaded != nil
                        ) {
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
        .task { await store.begin() }
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

/// Says only what is true. This card used to promise the photo was "safely in
/// the story" when the upload was the thing that failed, and that an offline
/// answer would arrive later when nothing was ever queued: closing the app
/// destroys the conversation.
struct DiagnosisErrorCard: View {
    let error: PlantyError
    let plantName: String
    /// Whether the photograph actually reached the service. The difference
    /// between "your picture is kept" and "take it again" is the whole message.
    let photoIsSaved: Bool
    let retry: () -> Void

    var body: some View {
        StateMessage(
            title: title,
            message: message,
            accent: PlantyColor.orange,
            icon: isOffline ? "wifi.slash" : "exclamationmark.bubble"
        ) {
            Button("Try again", action: retry)
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
        }
    }

    private var isOffline: Bool {
        if case .offline = error { return true }
        return false
    }

    private var title: String {
        isOffline ? "No connection, so there is no answer yet." : "Planty could not finish this comparison."
    }

    private var message: String {
        let fate = photoIsSaved
            ? "The photo is in \(plantName)'s story."
            : "The photo is still on your phone and has not been saved."
        return "\(fate) Nothing is waiting in the background, so try again when you can."
    }
}
