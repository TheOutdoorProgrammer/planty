import SwiftUI

struct PlantChatsScreen: View {
    @State var store: PlantChatsStore
    @Environment(AppSession.self) private var session
    @State private var destination: ResumedChat?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 14) {
                NavigationLink {
                    ConsultScreen(store: session.consultStore(for: store.plant))
                } label: {
                    Label("Start a new chat", systemImage: "plus.bubble.fill")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.pink))

                SectionHeading(
                    "Previous chats",
                    detail: "Every saved conversation about \(store.plant.commonName), newest first."
                )

                if store.isLoading && store.conversations.isEmpty {
                    ProgressView("Loading chats…")
                        .frame(maxWidth: .infinity, minHeight: 120)
                } else if store.conversations.isEmpty, store.error == nil {
                    StateMessage(
                        title: "No earlier chats yet",
                        message: "Start one above. It will stay here so you can pick it up later.",
                        accent: PlantyColor.cyan,
                        icon: "bubble.left.and.bubble.right"
                    ) { EmptyView() }
                } else {
                    ForEach(store.conversations) { conversation in
                        conversationButton(conversation)
                    }
                }

                if let error = store.error {
                    StateMessage(
                        title: "Planty could not load those chats.",
                        message: error.errorDescription ?? "Try again in a moment.",
                        accent: PlantyColor.orange,
                        icon: "exclamationmark.triangle.fill"
                    ) {
                        Button("Try again") { Task { await store.load() } }
                            .buttonStyle(SecondaryButtonStyle())
                    }
                }
            }
            .padding(16)
            .plantyReadableContent()
        }
        .plantyPage()
        .navigationTitle("Chats about \(store.plant.commonName)")
        .navigationBarTitleDisplayMode(.inline)
        .navigationDestination(isPresented: .init(
            get: { destination != nil },
            set: { if !$0 { destination = nil } }
        )) {
            if let destination {
                ConsultScreen(store: destination.store)
            }
        }
        .task { await store.load() }
        .refreshable { await store.load() }
    }

    private func conversationButton(_ conversation: PlantConversationSummary) -> some View {
        Button {
            Task {
                if case .success(let chat) = await store.resume(conversation) {
                    destination = ResumedChat(id: conversation.id, store: chat)
                }
            }
        } label: {
            VStack(alignment: .leading, spacing: 7) {
                HStack(alignment: .firstTextBaseline) {
                    Text(conversation.firstAsked)
                        .font(.headline)
                        .foregroundStyle(PlantyColor.foreground)
                        .lineLimit(2)
                    Spacer(minLength: 8)
                    if store.loadingConversationID == conversation.id {
                        ProgressView()
                    } else {
                        Image(systemName: "chevron.right")
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                }
                Text(conversation.latestReply)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
                    .lineLimit(2)
                Text("\(conversation.turnCount) \(conversation.turnCount == 1 ? "exchange" : "exchanges") · \(conversation.updatedAt.formatted(date: .abbreviated, time: .shortened))")
                    .font(.caption)
                    .foregroundStyle(PlantyColor.cyan)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .plantyCard(border: PlantyColor.cyan.opacity(0.18), padding: 14)
        }
        .buttonStyle(.plain)
        .disabled(store.loadingConversationID != nil)
        .accessibilityHint("Opens this conversation and continues it")
    }
}

private struct ResumedChat: Identifiable {
    let id: UUID
    let store: ConsultStore
}
