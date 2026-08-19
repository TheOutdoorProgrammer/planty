import SwiftUI

/// History as a story, top to bottom. Photos anchor time and short narrative
/// findings bridge them. Charts live behind a disclosure, never on top.
struct PlantStoryScreen: View {
    @State var store: PlantStoryStore
    @Environment(AppSession.self) private var session
    @State private var isEditing = false
    @State private var isLoggingCare = false
    @State private var isHarvesting = false
    @State private var isConfirmingDeath = false
    @State private var deathError: PlantyError?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                header
                currentState
                if let toxicity = store.toxicity {
                    PlantToxicitySection(plant: store.plant, toxicity: toxicity)
                }

                if let deathError {
                    deathCard(deathError)
                }
                if store.plant.status == .dead {
                    postmortemCard
                }

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
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button {
                        isEditing = true
                    } label: {
                        Label("Edit plant", systemImage: "pencil")
                    }
                    if !store.plant.status.isRetired {
                        Button(role: .destructive) {
                            isConfirmingDeath = true
                        } label: {
                            Label("It died\u{2026}", systemImage: "xmark.seal")
                        }
                    }
                } label: {
                    Image(systemName: "ellipsis.circle")
                }
                .accessibilityLabel("Plant actions")
            }
        }
        .sheet(isPresented: $isEditing) {
            EditPlantSheet(plant: store.plant) { patch in
                let failure = await store.saveEdits(patch)
                if failure == nil { session.library.apply(store.plant) }
                return failure
            }
        }
        .sheet(isPresented: $isLoggingCare) {
            CareLogSheet(plantName: store.plant.commonName) { kind, note in
                await store.record(kind, note: note)
            }
            .presentationDetents([.medium, .large])
        }
        .sheet(isPresented: $isHarvesting) {
            HarvestSheet(plantName: store.plant.commonName) { quantity, unit, notes in
                await store.logHarvest(quantity: quantity, unit: unit, notes: notes)
            }
            .presentationDetents([.medium, .large])
        }
        .confirmationDialog(
            "Record that \(store.plant.commonName) died?",
            isPresented: $isConfirmingDeath,
            titleVisibility: .visible
        ) {
            Button("It died", role: .destructive) {
                Task { await recordDeath() }
            }
            Button("Not yet", role: .cancel) {}
        } message: {
            Text("""
                The story and photos stay, and Planty can still say what went \
                wrong. But this cannot be undone from the app.
                """)
        }
        .safeAreaInset(edge: .bottom) {
            if !store.plant.status.isRetired {
                Button("Take today's photo") {
                    session.beginCapture(for: store.plant)
                }
                .buttonStyle(PrimaryButtonStyle())
                .padding(.horizontal, 20)
                .padding(.vertical, 10)
                .background(.ultraThinMaterial)
            }
        }
    }

    private func recordDeath() async {
        deathError = await store.markDead()
        if deathError == nil { session.library.apply(store.plant) }
    }

    /// Asking needs no photograph, which is why it sits here rather than
    /// behind the camera: most questions are about the record, not a picture.
    private var plantActions: some View {
        HStack(spacing: 10) {
            NavigationLink {
                ConsultScreen(store: session.consultStore(for: store.plant))
            } label: {
                ActionFace("Ask", icon: "bubble.left.and.text.bubble.right.fill")
            }
            .buttonStyle(SecondaryButtonStyle())

            NavigationLink {
                RemindersScreen(store: session.remindersStore(for: store.plant))
            } label: {
                ActionFace("Reminders", icon: "bell.fill")
            }
            .buttonStyle(SecondaryButtonStyle())

            NavigationLink {
                NotesScreen(store: session.notesStore(for: store.plant))
            } label: {
                ActionFace("Notes", icon: "note.text")
            }
            .buttonStyle(SecondaryButtonStyle())
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
            plantActions
            if !store.plant.status.isRetired {
                careActions
            }
            shelterControl
        }
    }

    /// Care gets recorded where the plant lives, not by telling the chat. The
    /// harvest button only exists where a harvest can.
    private var careActions: some View {
        HStack(spacing: 10) {
            Button {
                isLoggingCare = true
            } label: {
                Label("Log care", systemImage: "checkmark.circle.fill")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(SecondaryButtonStyle())
            .accessibilityLabel("Log care for \(store.plant.commonName)")

            if isEdible {
                Button {
                    isHarvesting = true
                } label: {
                    Label("Harvest", systemImage: "basket.fill")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(SecondaryButtonStyle())
                .accessibilityLabel("Log a harvest from \(store.plant.commonName)")
            }
        }
    }

    private var isEdible: Bool {
        store.plant.domain == .edibleIndoor || store.plant.domain == .edibleOutdoor
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

    /// The lesson gets the loudest type in the card: it is the reason a dead
    /// plant keeps its record at all.
    private var postmortemCard: some View {
        VStack(alignment: .leading, spacing: 12) {
            Eyebrow(text: "Postmortem", color: PlantyColor.purple)

            if let postmortem = store.postmortem {
                Text(postmortem.likelyCause)
                    .font(.headline)
                Text(postmortem.narrative)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
                if let lesson = postmortem.lesson {
                    Label("Next time", systemImage: "lightbulb.fill")
                        .font(.caption.weight(.bold))
                        .foregroundStyle(PlantyColor.yellow)
                        .padding(.top, 4)
                    Text(lesson)
                        .font(.title3.weight(.bold))
                }
            } else if store.isAskingPostmortem {
                HStack(alignment: .top, spacing: 12) {
                    ProgressView()
                    Text("""
                        Planty is reading the whole story back before it \
                        answers. This can take a minute.
                        """)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
                }
                .accessibilityElement(children: .combine)
            } else {
                Text("\(store.plant.commonName) is recorded as dead. The story above stays.")
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
                if let detail = store.postmortemError?.errorDescription {
                    Text(detail)
                        .font(.footnote)
                        .foregroundStyle(PlantyColor.orange)
                }
                Button(store.postmortemError == nil ? "Ask what killed it" : "Ask again") {
                    Task { await store.askPostmortem() }
                }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.purple))
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: PlantyColor.purple.opacity(0.4))
    }

    private func deathCard(_ failure: PlantyError) -> some View {
        StateMessage(
            title: "The death was not recorded.",
            message: failure.errorDescription ?? "The service could not be reached.",
            accent: PlantyColor.orange,
            icon: "exclamationmark.triangle.fill"
        ) {
            Button("Try again") { Task { await recordDeath() } }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
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
