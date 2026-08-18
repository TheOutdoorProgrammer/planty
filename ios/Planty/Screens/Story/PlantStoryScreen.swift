import SwiftUI

/// History as a story, top to bottom. Photos anchor time and short narrative
/// findings bridge them. Charts live behind a disclosure, never on top.
struct PlantStoryScreen: View {
    @State var store: PlantStoryStore
    @Environment(AppSession.self) private var session

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                header
                currentState

                if let error = store.error {
                    chapterErrorCard(error)
                }

                if store.hasStory {
                    ForEach(store.chapters) { chapter in
                        ChapterRow(chapter: chapter, plant: store.plant)
                    }
                } else if store.hasLoaded {
                    emptyStory
                }

                WhyPlantyThinksThis(series: store.series, verdict: store.verdict)
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 20)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .plantyPage()
        .navigationTitle(store.plant.commonName)
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await store.load() }
        .task { await store.load() }
        .safeAreaInset(edge: .bottom) {
            Button("Take today's photo") {
                session.beginCapture(for: store.plant)
            }
            .buttonStyle(PrimaryButtonStyle())
            .padding(.horizontal, 20)
            .padding(.vertical, 10)
            .background(.ultraThinMaterial)
        }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(store.plant.commonName)
                .font(.largeTitle.weight(.bold))
            HStack(spacing: 10) {
                if let species = store.plant.displaySpecies {
                    Text(species)
                        .font(.subheadline)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
                OwnershipBadge(plant: store.plant)
            }
            PlantPhotoView(
                plant: store.plant,
                photo: store.chapters.first?.photo,
                height: 240
            )
            if comparison.isPossible {
                NavigationLink("Compare with the first photo") {
                    PhotoComparisonScreen(plant: store.plant, comparison: comparison)
                }
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(PlantyColor.green)
            }
            shelterControl
        }
    }

    /// Only a plant with a cold threshold was ever asked to come in. The row
    /// states where it is now and offers the other side, because the warning
    /// repeats until somebody says it was answered.
    @ViewBuilder
    private var shelterControl: some View {
        if store.plant.canShelter {
            HStack(spacing: 12) {
                Label(
                    store.plant.isSheltered ? "Indoors for the cold" : "Outside",
                    systemImage: store.plant.isSheltered ? "house.fill" : "sun.max.fill"
                )
                .font(.caption)
                .foregroundStyle(PlantyColor.secondaryText)

                Spacer()

                Button(store.plant.isSheltered ? "Back outside" : "Brought indoors") {
                    Task { await store.setSheltered(!store.plant.isSheltered) }
                }
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(PlantyColor.green)
            }
        }
    }

    private var comparison: PhotoComparison {
        PhotoComparison(store.timeline.photos)
    }

    /// Says what Planty concluded and how fresh that is, and never claims the
    /// plant is healthy.
    private var currentState: some View {
        VStack(alignment: .leading, spacing: 8) {
            StatusPill(state: store.careState)
            Text(headline)
                .font(.title3.weight(.bold))
            Text(store.verdict?.reasoning ?? store.careState.sentence)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
            Text(store.lastComparedLine)
                .font(.footnote)
                .foregroundStyle(
                    store.freshness.isFresh ? PlantyColor.green : PlantyColor.yellow
                )
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: store.careState.color.opacity(0.4))
    }

    private var headline: String {
        switch store.careState {
        case .allGood: "Doing okay"
        case .watch: "Watching one thing"
        case .needsCare: "One thing to do"
        case .urgent: "Needs you now"
        case .unknown: "Planty cannot say yet"
        }
    }

    private var emptyStory: some View {
        StateMessage(
            title: store.hasPhotos
                ? "Nothing has happened yet. Nice."
                : "\(store.plant.commonName) has data, but no story yet.",
            message: store.hasPhotos
                ? "Photos, care actions, and useful changes will collect here over time."
                : """
                    Sensors can tell Planty about the pot and the room. A photo \
                    adds the part they cannot see.
                    """,
            accent: PlantyColor.pink,
            icon: "camera.fill"
        ) {
            Button("Take the first photo") { session.beginCapture(for: store.plant) }
                .buttonStyle(PrimaryButtonStyle())
        }
    }

    private func chapterErrorCard(_ error: PlantyError) -> some View {
        StateMessage(
            title: "The newest chapter is missing.",
            message: """
                Earlier photos and notes are still available. Planty could not \
                load the latest events.
                """,
            accent: PlantyColor.orange,
            icon: "arrow.trianglehead.clockwise"
        ) {
            Button("Try again") { Task { await store.load() } }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
        }
    }
}

struct ChapterRow: View {
    let chapter: StoryChapter
    let plant: Plant

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Eyebrow(text: chapter.dateLabel, color: PlantyColor.secondaryText)
            Text(chapter.title)
                .font(.headline)

            if let photo = chapter.photo {
                PlantPhotoView(plant: plant, photo: photo, height: 200)
            }

            Text(chapter.narrative)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)

            ForEach(chapter.events) { event in
                if event.photo == nil {
                    Label(event.title, systemImage: event.symbol)
                        .font(.footnote)
                        .foregroundStyle(event.careState?.color ?? PlantyColor.secondaryText)
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard()
        .accessibilityElement(children: .contain)
    }
}
