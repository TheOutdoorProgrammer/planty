import SwiftUI

/// Ask Planty something. With a plant the record is the subject and pictures
/// are optional; with none it is a scratch chat that files nothing anywhere.
struct ConsultScreen: View {
    @Environment(AppSession.self) private var session
    @State var store: ConsultStore
    @FocusState private var composerFocused: Bool
    @State private var isAttaching = false

    var body: some View {
        ScrollViewReader { proxy in
            ScrollView {
                VStack(alignment: .leading, spacing: 16) {
                    if !store.hasStarted {
                        opening
                    }
                    historyCapabilityWarning
                    ForEach(store.messages) { message in
                        row(message)
                            .id(message.id)
                    }
                    if store.isThinking {
                        ThinkingRow(stageLine: store.thinkingLine)
                    }
                    if let error = store.error {
                        askError(error)
                    }
                    followUps
                }
                .padding(.horizontal, 20)
                .padding(.vertical, 16)
                .plantyReadableContent()
            }
            .onChange(of: store.messages.count) { _, _ in
                guard let last = store.messages.last else { return }
                proxy.scrollTo(last.id, anchor: .bottom)
            }
        }
        .plantyPage()
        .navigationTitle(store.title)
        .navigationBarTitleDisplayMode(.inline)
        .safeAreaInset(edge: .bottom) { composer }
        .task { await store.begin() }
        .task { await session.models.loadIfNeeded() }
        .sheet(isPresented: $isAttaching) {
            PhotoAttachSheet { jpeg in store.attach(jpeg: jpeg) }
        }
    }

    /// An empty chat is a blank stare, so it opens with what it already knows
    /// and three questions it can actually answer.
    private var opening: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(openingTitle)
                .font(.headline)
                .foregroundStyle(PlantyColor.foreground)

            Text(openingBody)
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

    private var openingTitle: String { store.openingTitle }

    private var openingBody: String { store.openingBody }

    @ViewBuilder
    private var historyCapabilityWarning: some View {
        if store.plant != nil,
           session.models.hasLoaded,
           session.models.canInspectHistoricalPhotos(for: .consult) == false {
            VStack(alignment: .leading, spacing: 8) {
                Label("Older photos are unavailable to this model", systemImage: "photo.badge.exclamationmark")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(PlantyColor.yellow)
                Text("It can use the written record and a photo attached to this message, but it cannot inspect the timeline on demand.")
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
                Button("Choose another model") { session.isShowingSettings = true }
                    .buttonStyle(SecondaryButtonStyle())
            }
            .plantyCard(border: PlantyColor.yellow.opacity(0.3), padding: 14)
        }
    }

    @ViewBuilder
    private func row(_ message: ConsultMessage) -> some View {
        switch message.speaker {
        case .user:
            userBubble(message)
        case .planty:
            answerCard(message)
        }
    }

    /// The photo is shown next to the words it was sent with, so nobody has to
    /// remember which picture a question was about.
    private func userBubble(_ message: ConsultMessage) -> some View {
        HStack {
            Spacer(minLength: 40)
            VStack(alignment: .trailing, spacing: 8) {
                if let jpeg = message.photo, let image = UIImage(data: jpeg) {
                    Image(uiImage: image)
                        .resizable()
                        .scaledToFill()
                        .frame(maxWidth: 220, maxHeight: 220)
                        .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
                        .accessibilityLabel("Photo you sent")
                } else if message.photoID != nil {
                    Label("Photo attached", systemImage: "photo.fill")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(PlantyColor.background)
                        .padding(.horizontal, 12)
                        .padding(.vertical, 8)
                        .background(PlantyColor.pink, in: Capsule())
                }
                if !message.text.isEmpty {
                    Text(message.text)
                        .font(.body)
                        .foregroundStyle(PlantyColor.background)
                        .padding(.horizontal, 14)
                        .padding(.vertical, 10)
                        .background(PlantyColor.pink, in: RoundedRectangle(cornerRadius: 18))
                }
            }
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

            if let answer = message.answer, !answer.steps.isEmpty {
                Divider().overlay(PlantyColor.quietDecoration)
                StepsDisclosure(steps: answer.steps)
            }
        }
        .plantyCard(padding: 14)
        // Not .combine: the trace below is interactive, and combining would
        // flatten its buttons into the answer text and make them unreachable.
        .accessibilityElement(children: .contain)
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
                if let attempt = store.failed {
                    if attempt.photo != nil {
                        Text("Your photo is still here. It was not sent anywhere.")
                            .font(.footnote)
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                    Button("Ask again") {
                        Task { await store.retry() }
                    }
                    .buttonStyle(PrimaryButtonStyle())

                    Button(attempt.photo == nil ? "Edit the question" : "Edit it and keep the photo") {
                        store.recoverDraft()
                    }
                    .buttonStyle(SecondaryButtonStyle())
                } else if store.canResumePendingReply {
                    Button("Check for reply") {
                        Task { await store.resumePendingReply() }
                    }
                    .buttonStyle(PrimaryButtonStyle())
                } else {
                    Button("Dismiss") { store.clearError() }
                        .buttonStyle(SecondaryButtonStyle())
                }
            }
        }
    }

    private var composer: some View {
        VStack(spacing: 10) {
            if let jpeg = store.attachment {
                attached(jpeg)
            }
            HStack(spacing: 10) {
                Button {
                    composerFocused = false
                    isAttaching = true
                } label: {
                    Image(systemName: "camera.fill")
                        .font(.title3)
                        .foregroundStyle(PlantyColor.cyan)
                }
                .frame(minWidth: 44, minHeight: 44)
                .accessibilityLabel("Add a photo to this message")

                TextField(composerPrompt, text: $store.composer, axis: .vertical)
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
                .disabled(!store.canSend)
            }
        }
        .padding(.horizontal, 20)
        .padding(.vertical, 10)
        .background(.ultraThinMaterial)
    }

    private var composerPrompt: String {
        store.attachment == nil
            ? (store.plant == nil ? "Ask about anything…" : "Ask about this plant…")
            : "Say something about the photo, or just send it"
    }

    /// Shown before it goes, with a way out: an attachment nobody can see or
    /// remove is a photo you send by accident.
    private func attached(_ jpeg: Data) -> some View {
        HStack(spacing: 12) {
            if let image = UIImage(data: jpeg) {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFill()
                    .frame(width: 56, height: 56)
                    .clipShape(RoundedRectangle(cornerRadius: 12, style: .continuous))
            }
            Text("Photo attached to your next message.")
                .font(.footnote)
                .foregroundStyle(PlantyColor.secondaryText)
            Spacer(minLength: 8)
            Button {
                store.removeAttachment()
            } label: {
                Image(systemName: "xmark.circle.fill")
                    .font(.title3)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            .frame(minWidth: 44, minHeight: 44)
            .accessibilityLabel("Remove the attached photo")
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
