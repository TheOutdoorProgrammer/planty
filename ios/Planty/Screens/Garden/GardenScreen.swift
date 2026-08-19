import SwiftUI

/// Work that spans the whole collection: people, weather, travel, and history.
struct GardenScreen: View {
    @Environment(AppSession.self) private var session

    private var store: GardenStore { session.garden }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    if !store.isConfigured {
                        StateMessage(
                            title: "Connect Planty first",
                            message: "Garden planning needs the Planty service.",
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
                            title: "The garden did not load",
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
                        routes
                    }
                }
                .padding(.horizontal, 20)
                .padding(.vertical, 24)
            }
            .plantyPage()
            .navigationTitle("Garden")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button { session.isShowingSettings = true } label: {
                        Image(systemName: "person.crop.circle")
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
        }
    }

    private var routes: some View {
        VStack(spacing: 14) {
            NavigationLink {
                QuestionsScreen(store: store, plants: session.library.plants)
            } label: {
                GardenRouteCard(
                    eyebrow: "People",
                    title: "Questions",
                    detail: store.questions.isEmpty
                        ? "Nothing waiting on an answer"
                        : "(store.questions.count) waiting on an answer",
                    symbol: "person.2.wave.2.fill",
                    color: PlantyColor.cyan
                )
            }

            NavigationLink {
                ColdWatchScreen(store: store)
            } label: {
                GardenRouteCard(
                    eyebrow: "Weather",
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
                    eyebrow: "Travel",
                    title: "Plan time away",
                    detail: "Tell Planty who can cover the garden",
                    symbol: "suitcase.rolling.fill",
                    color: PlantyColor.orange
                )
            }

            NavigationLink {
                GardenHistoryScreen(store: store)
            } label: {
                GardenRouteCard(
                    eyebrow: "Memory",
                    title: "Garden history",
                    detail: "Harvests and lessons from plants you lost",
                    symbol: "book.pages.fill",
                    color: PlantyColor.green
                )
            }
        }
        .buttonStyle(.plain)
    }
}

private struct PartialGardenWarning: View {
    let error: PlantyError
    let retry: () -> Void
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        Group {
            if dynamicTypeSize.isAccessibilitySize {
                VStack(alignment: .leading, spacing: 12) { warningContent }
            } else {
                HStack(spacing: 12) { warningContent }
            }
        }
        .plantyCard(border: PlantyColor.orange.opacity(0.35), padding: 14)
    }

    @ViewBuilder
    private var warningContent: some View {
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
            if !dynamicTypeSize.isAccessibilitySize {
                Spacer(minLength: 4)
            }
            Button("Retry", action: retry)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(PlantyColor.orange)
    }
}

private struct GardenHero: View {
    let questions: Int
    let harvests: Int
    let lessons: Int
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            if dynamicTypeSize.isAccessibilitySize {
                VStack(alignment: .leading, spacing: 14) { intro }
            } else {
                HStack(alignment: .top, spacing: 16) { intro }
            }

            if dynamicTypeSize.isAccessibilitySize {
                VStack(spacing: 10) { metrics }
            } else {
                HStack(spacing: 10) { metrics }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: PlantyColor.green.opacity(0.4))
    }

    @ViewBuilder
    private var intro: some View {
        ZStack {
            Circle()
                .fill(PlantyColor.green.opacity(0.15))
                .frame(width: 72, height: 72)
            Circle()
                .stroke(PlantyColor.cyan.opacity(0.45), lineWidth: 2)
                .frame(width: 54, height: 54)
            Image(systemName: "leaf.fill")
                .font(.title.weight(.semibold))
                .foregroundStyle(PlantyColor.green)
        }
        .accessibilityHidden(true)

        VStack(alignment: .leading, spacing: 6) {
            Eyebrow(text: "The whole garden", color: PlantyColor.green)
            Text("Plan past today")
                .font(.title2.weight(.bold))
            Text("Trips, cold nights, questions, and what the garden taught you.")
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
        }
    }

    @ViewBuilder
    private var metrics: some View {
        GardenMetric(value: questions, label: "open")
        GardenMetric(value: harvests, label: "harvests")
        GardenMetric(value: lessons, label: "lessons")
    }
}

private struct GardenMetric: View {
    let value: Int
    let label: String
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        Group {
            if dynamicTypeSize.isAccessibilitySize {
                HStack(spacing: 12) { content }
            } else {
                VStack(spacing: 2) { content }
            }
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, 10)
        .background(PlantyColor.background.opacity(0.45), in: RoundedRectangle(cornerRadius: 14))
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private var content: some View {
            Text(value.formatted())
                .font(.title3.monospacedDigit().weight(.bold))
            Text(label)
                .font(.caption)
                .foregroundStyle(PlantyColor.secondaryText)
    }
}

private struct GardenRouteCard: View {
    let eyebrow: String
    let title: String
    let detail: String
    let symbol: String
    let color: Color
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    var body: some View {
        Group {
            if dynamicTypeSize.isAccessibilitySize {
                VStack(alignment: .leading, spacing: 12) { routeContent }
            } else {
                HStack(spacing: 15) { routeContent }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: color.opacity(0.28), padding: 16)
        .contentShape(RoundedRectangle(cornerRadius: 24, style: .continuous))
    }

    @ViewBuilder
    private var routeContent: some View {
            Image(systemName: symbol)
                .font(.title2.weight(.semibold))
                .foregroundStyle(color)
                .frame(width: 42, height: 42)
                .background(color.opacity(0.13), in: Circle())
                .accessibilityHidden(true)
            VStack(alignment: .leading, spacing: 3) {
                Eyebrow(text: eyebrow, color: color)
                Text(title)
                    .font(.headline)
                    .foregroundStyle(PlantyColor.foreground)
                Text(detail)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
                    .multilineTextAlignment(.leading)
            }
            if !dynamicTypeSize.isAccessibilitySize {
                Spacer(minLength: 8)
                Image(systemName: "chevron.right")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(PlantyColor.secondaryText)
            }
    }
}

private struct GardenLoadingView: View {
    var body: some View {
        VStack(spacing: 16) {
            ProgressView().tint(PlantyColor.green)
            Text("Gathering the garden…")
                .font(.headline)
            Text("Questions, harvests, and lessons are coming together.")
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .frame(maxWidth: .infinity)
        .plantyCard(border: PlantyColor.green.opacity(0.35))
    }
}
