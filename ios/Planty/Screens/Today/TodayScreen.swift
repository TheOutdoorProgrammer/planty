import SwiftUI

/// Orientation before action. Designed from the calm state outward: the most
/// common screen is a completion, not an empty list.
struct TodayScreen: View {
    @Environment(AppSession.self) private var session

    private var store: TodayStore { session.today }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    if let release = session.updates.available {
                        UpdateBanner(release: release) { session.updates.dismiss() }
                    }
                    content

                    // Below the day's actions: a question for an absent friend
                    // is worth surfacing, but never ahead of a thirsty plant.
                    OpenQuestionsCard(questions: store.openQuestions) { question, answer in
                        await store.answer(question, with: answer)
                    }
                }
                .padding(.horizontal, 20)
                .padding(.vertical, 24)
                .frame(maxWidth: .infinity, alignment: .leading)
            }
            .plantyPage()
            .navigationTitle("Planty")
            .toolbar { profileButton }
            .refreshable { await store.load() }
            .task { await store.load() }
            .task { await session.updates.check() }
            .onChange(of: session.capture.settled) { _, verdict in
                guard let verdict else { return }
                store.settled(verdict)
                session.capture.clearSettled()
            }
            .alert(
                "That did not save.",
                isPresented: Binding(
                    get: { store.actionError != nil },
                    set: { if !$0 { store.clearActionError() } }
                ),
                presenting: store.actionError
            ) { _ in
                Button("OK") { store.clearActionError() }
            } message: { failure in
                Text(failure.errorDescription ?? "Try again in a moment.")
            }
            .overlay(alignment: .bottom) {
                if let noted = store.noted {
                    SaveToast(message: "Noted on \(noted).")
                        .task {
                            try? await Task.sleep(for: .seconds(2))
                            store.clearNoted()
                        }
                }
            }
        }
    }

    @ViewBuilder
    private var content: some View {
        switch store.presentation {
        case .unconfigured:
            UnconfiguredCard { session.isShowingSettings = true }
        case .loadingCold:
            LoadingColdView()
        case .loadingWarm(let previous, let checkedAt):
            LoadingWarmView(checkedAt: checkedAt)
            cards(for: previous.sortedEntries)
        case .emptySetup:
            EmptySetupView()
        case .calm(let summary):
            CalmHero(summary: summary) { session.selectedTab = .snap }
        case .actions(let summary):
            actionState(summary)
        case .stale(let summary):
            StaleBanner(summary: summary) {
                session.isShowingSettings = true
            } takePhoto: {
                session.selectedTab = .snap
            }
            cards(for: summary.pending)
        case .failed(let error, let cached):
            TodayErrorView(error: error) {
                Task { await store.load() }
            } takePhoto: {
                session.selectedTab = .snap
            }
            if let cached {
                cards(for: cached.sortedEntries)
            }
        }
    }

    @ViewBuilder
    private func actionState(_ summary: ActionSummary) -> some View {
        Text(summary.headline)
            .font(.title.weight(.bold))
            .foregroundStyle(PlantyColor.foreground)

        cards(for: summary.featured)

        if let deferredLabel = summary.deferredLabel {
            DisclosureGroup(deferredLabel) {
                cards(for: summary.deferred)
                    .padding(.top, 8)
            }
            .tint(PlantyColor.secondaryText)
            .font(.subheadline.weight(.semibold))
        }

        Text(summary.footnote)
            .font(.subheadline)
            .foregroundStyle(PlantyColor.secondaryText)
    }

    @ViewBuilder
    private func cards(for entries: [DigestEntry]) -> some View {
        ForEach(entries) { entry in
            NavigationLink {
                TodayActionScreen(entry: entry)
            } label: {
                CareCard(entry: entry)
            }
            .buttonStyle(.plain)
        }
    }

    private var profileButton: some ToolbarContent {
        ToolbarItem(placement: .topBarTrailing) {
            Button {
                session.isShowingSettings = true
            } label: {
                Image(systemName: "person.crop.circle")
            }
            .accessibilityLabel("Settings, sensors and data freshness")
        }
    }
}
