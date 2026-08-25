import SwiftUI

/// Collection-wide tools live under More so the bottom navigation speaks in
/// tasks users recognize. No Garden feature is removed; this is only a clearer
/// doorway into questions, weather, travel, history, and owner updates.
struct GardenScreen: View {
    @Environment(AppSession.self) private var session
    @State private var ownerUpdateTarget: OwnerUpdateTarget?

    private var store: GardenStore { session.garden }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    if !store.isConfigured {
                        StateMessage(
                            title: "Connect Planty first",
                            message: "Planning tools need the Planty service.",
                            accent: PlantyColor.orange,
                            icon: "link.badge.plus"
                        ) {
                            Button("Open settings") { session.isShowingSettings = true }
                                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
                        }
                    } else if store.isLoading && !store.hasLoaded {
                        GardenLoadingView()
                    } else if let error = store.error, !store.hasLoaded {
                        StateMessage(
                            title: "These tools did not load",
                            message: error.errorDescription ?? "Try again in a moment.",
                            accent: PlantyColor.orange,
                            icon: "wifi.exclamationmark"
                        ) {
                            Button("Try again") { Task { await store.load() } }
                                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
                        }
                    } else {
                        GardenHero(
                            questions: store.questions.count,
                            harvests: store.harvests.count,
                            lessons: store.postmortems.count
                        )
                        if let error = store.error {
                            PartialGardenWarning(error: error) {
                                Task { await store.load() }
                            }
                        }
                        peopleRoutes
                        routes
                        settingsRoute
                    }
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 16)
                .plantyReadableContent()
            }
            .plantyPage()
            .navigationTitle("More")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button { session.isShowingSettings = true } label: {
                        Image(systemName: "gearshape.fill")
                    }
                    .accessibilityLabel("Settings")
                }
            }
            .refreshable { await store.load() }
            .task {
                async let gardenLoad: Void = store.load()
                async let plantLoad: Void = session.library.load()
                _ = await (gardenLoad, plantLoad)
            }
            .sheet(item: $ownerUpdateTarget) { target in
                OwnerUpdateFlow(steward: target.name, store: store)
            }
        }
    }

    @ViewBuilder
    private var peopleRoutes: some View {
        if !friendStewards.isEmpty {
            VStack(alignment: .leading, spacing: 12) {
                SectionHeading(
                    "People",
                    detail: "Send the person whose plants you're watching a real update."
                )
                ForEach(friendStewards, id: \.self) { steward in
                    Button {
                        ownerUpdateTarget = OwnerUpdateTarget(name: steward)
                    } label: {
                        GardenRouteCard(
                            title: "Update \(steward)",
                            detail: ownerDetail(steward),
                            symbol: "message.fill",
                            color: PlantyColor.purple
                        )
                    }
                    .buttonStyle(.plain)
                }
            }
        }
    }

    private var routes: some View {
        VStack(alignment: .leading, spacing: 12) {
            SectionHeading("Garden tools", detail: "Things that span more than one plant.")

            NavigationLink {
                QuestionsScreen(store: store, plants: session.library.plants)
            } label: {
                GardenRouteCard(
                    title: "Questions",
                    detail: store.questions.isEmpty
                        ? "Nothing waiting on an answer"
                        : "\(store.questions.count) waiting on an answer",
                    symbol: "person.2.wave.2.fill",
                    color: PlantyColor.cyan
                )
            }

            NavigationLink {
                ColdWatchScreen(store: store)
            } label: {
                GardenRouteCard(
                    title: "Cold watch",
                    detail: "See what needs shelter before a cold night",
                    symbol: "thermometer.snowflake",
                    color: PlantyColor.purple
                )
            }

            NavigationLink {
                AwayPlannerScreen(store: store)
            } label: {
                GardenRouteCard(
                    title: "Plan time away",
                    detail: "Set dates and tell Planty who can cover the garden",
                    symbol: "suitcase.rolling.fill",
                    color: PlantyColor.orange
                )
            }

            NavigationLink {
                GardenHistoryScreen(store: store)
            } label: {
                GardenRouteCard(
                    title: "Garden history",
                    detail: "Harvests and lessons from plants you lost",
                    symbol: "book.pages.fill",
                    color: PlantyColor.green
                )
            }
        }
        .buttonStyle(.plain)
    }

    private var settingsRoute: some View {
        VStack(alignment: .leading, spacing: 12) {
            SectionHeading("App")
            Button {
                session.isShowingSettings = true
            } label: {
                GardenRouteCard(
                    title: "Settings",
                    detail: "Connection, sensors, data freshness, and app configuration",
                    symbol: "gearshape.fill",
                    color: PlantyColor.secondaryText
                )
            }
            .buttonStyle(.plain)
        }
    }

    private var friendStewards: [String] {
        Array(Set(session.library.plants.filter(\.isFriends).map(\.steward))).sorted()
    }

    private func ownerDetail(_ steward: String) -> String {
        let count = session.library.plants.filter { $0.isFriends && $0.steward == steward }.count
        let noun = count == 1 ? "plant" : "plants"
        return "Summarize the last 7 days for \(count) \(noun) and attach the latest photos"
    }
}

private struct PartialGardenWarning: View {
    let error: PlantyError
    let retry: () -> Void

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: "exclamationmark.arrow.triangle.2.circlepath")
                .foregroundStyle(PlantyColor.orange)
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 2) {
                Text("Some garden records are unavailable")
                    .font(.subheadline.weight(.semibold))
                Text(error.errorDescription ?? "Try again in a moment.")
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            Spacer(minLength: 4)
            Button("Retry", action: retry)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(PlantyColor.orange)
                .frame(minHeight: 44)
        }
        .plantyCard(border: PlantyColor.orange.opacity(0.2), padding: 14)
    }
}

private struct GardenHero: View {
    let questions: Int
    let harvests: Int
    let lessons: Int

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(alignment: .top, spacing: 12) {
                Image(systemName: "leaf.circle.fill")
                    .font(.largeTitle)
                    .foregroundStyle(PlantyColor.green)
                    .accessibilityHidden(true)
                VStack(alignment: .leading, spacing: 5) {
                    Text("Beyond today's care")
                        .font(.title2.weight(.bold))
                    Text("Plan around weather and travel, answer people, and keep the garden's memory.")
                        .font(.subheadline)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            }

            HStack(spacing: 8) {
                GardenMetric(value: questions, label: "open")
                GardenMetric(value: harvests, label: "harvests")
                GardenMetric(value: lessons, label: "lessons")
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: PlantyColor.green.opacity(0.18))
    }
}

private struct GardenMetric: View {
    let value: Int
    let label: String

    var body: some View {
        VStack(spacing: 2) {
            Text(value.formatted())
                .font(.headline.monospacedDigit())
            Text(label)
                .font(.caption2)
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 9)
        .background(PlantyColor.elevated, in: RoundedRectangle(cornerRadius: 12))
        .accessibilityElement(children: .combine)
    }
}

private struct GardenRouteCard: View {
    let title: String
    let detail: String
    let symbol: String
    let color: Color

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: symbol)
                .font(.headline.weight(.semibold))
                .foregroundStyle(color)
                .frame(width: 40, height: 40)
                .background(color.opacity(0.1), in: Circle())
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 3) {
                Text(title)
                    .font(.headline)
                    .foregroundStyle(PlantyColor.foreground)
                Text(detail)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
                    .multilineTextAlignment(.leading)
            }
            Spacer(minLength: 8)
            Image(systemName: "chevron.right")
                .font(.caption.weight(.bold))
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: color.opacity(0.14), padding: 14)
        .contentShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
    }
}

private struct GardenLoadingView: View {
    var body: some View {
        HStack(spacing: 14) {
            ProgressView().tint(PlantyColor.green)
            VStack(alignment: .leading, spacing: 3) {
                Text("Loading garden tools…")
                    .font(.headline)
                Text("Questions, harvests, and lessons are coming together.")
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard()
    }
}
