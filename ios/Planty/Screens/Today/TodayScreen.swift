import SwiftUI

/// The landing screen answers the only question that matters first: is there
/// anything to do? Secondary destinations stay visible, but below that answer.
struct TodayScreen: View {
    @Environment(AppSession.self) private var session
    @State private var deferredExpanded = false

    private var store: TodayStore { session.today }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    if let release = session.updates.available {
                        UpdateBanner(release: release) { session.updates.dismiss() }
                    }

                    content

                    OpenQuestionsCard(questions: store.openQuestions) { question, answer in
                        await store.answer(question, with: answer)
                    }

                    shortcuts
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 18)
                .frame(maxWidth: .infinity, alignment: .leading)
                .plantyReadableContent()
            }
            .plantyPage()
            .navigationTitle("Today")
            .toolbar { settingsButton }
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
            reminderSection(previous.sortedDueReminders)
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
            reminderSection(summary.reminders)
            cards(for: summary.pending)
        case .failed(let error, let cached):
            TodayErrorView(error: error) {
                Task { await store.load() }
            } takePhoto: {
                session.selectedTab = .snap
            }
            if let cached {
                reminderSection(cached.sortedDueReminders)
                cards(for: cached.sortedEntries)
            }
        }
    }

    @ViewBuilder
    private func actionState(_ summary: ActionSummary) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(summary.headline, systemImage: "hand.raised.fill")
                .font(.title2.weight(.bold))
                .foregroundStyle(PlantyColor.foreground)
            Text(summary.footnote)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: PlantyColor.orange.opacity(0.22))

        reminderSection(summary.reminders)

        if !summary.featured.isEmpty {
            SectionHeading("Needs attention")
            cards(for: summary.featured)
        }

        if let deferredLabel = summary.deferredLabel {
            VStack(alignment: .leading, spacing: 0) {
                PlantyDisclosureHeader(
                    title: deferredLabel,
                    icon: "clock.arrow.circlepath",
                    isExpanded: $deferredExpanded
                )
                .accessibilityIdentifier("today.deferred.toggle")

                if deferredExpanded {
                    VStack(spacing: 12) {
                        cards(for: summary.deferred)
                    }
                    .padding(.top, 12)
                }
            }
            .plantyCard(padding: 14)
        }
    }

    @ViewBuilder
    private func reminderSection(_ reminders: [DueReminder]) -> some View {
        if !reminders.isEmpty {
            SectionHeading(reminders.count == 1 ? "Due reminder" : "Due reminders")
            reminderCards(for: reminders)
        }
    }

    @ViewBuilder
    private func reminderCards(for reminders: [DueReminder]) -> some View {
        ForEach(reminders) { occurrence in
            TodayReminderCard(
                occurrence: occurrence,
                isResolving: store.resolvingReminderOccurrenceIDs.contains(
                    occurrence.occurrenceID
                )
            ) { disposition in
                Task { await store.resolve(occurrence, as: disposition) }
            }
        }
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

    private var shortcuts: some View {
        VStack(alignment: .leading, spacing: 12) {
            SectionHeading("Quick access")
            ViewThatFits(in: .horizontal) {
                HStack(spacing: 10) {
                    takePhotoShortcut
                    plantsShortcut
                }
                VStack(spacing: 10) {
                    takePhotoShortcut
                    plantsShortcut
                }
            }
        }
    }

    private var takePhotoShortcut: some View {
        Button { session.selectedTab = .snap } label: {
            ActionFace("Take photo", icon: "camera.fill")
        }
        .buttonStyle(SecondaryButtonStyle())
    }

    private var plantsShortcut: some View {
        Button { session.selectedTab = .plants } label: {
            ActionFace("Plants", icon: "leaf.fill")
        }
        .buttonStyle(SecondaryButtonStyle())
    }

    private var settingsButton: some ToolbarContent {
        ToolbarItem(placement: .topBarTrailing) {
            Button {
                session.isShowingSettings = true
            } label: {
                Image(systemName: "gearshape.fill")
            }
            .accessibilityLabel("Settings, sensors and data freshness")
        }
    }
}
