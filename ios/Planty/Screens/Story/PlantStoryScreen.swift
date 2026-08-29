import SwiftUI

/// A plant page starts with identity, current recommendation, and the actions a
/// person can take while standing beside it. Facts and history come after the
/// decision instead of competing with it.
struct PlantStoryScreen: View {
    @State var store: PlantStoryStore
    @Environment(AppSession.self) private var session
    @State private var isEditing = false
    @State private var isEditingToxicity = false
    @State private var isLoggingCare = false
    @State private var isManagingAirflow = false
    @State private var isHarvesting = false
    @State private var isConfirmingDeath = false
    @State private var isConfirmingArchive = false
    @State private var isShowingLabel = false
    @State private var deletingPhoto: Photo?
    @State private var actionError: PlantyError?
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    private var actionColumns: [GridItem] {
        dynamicTypeSize.isAccessibilitySize
            ? [GridItem(.flexible())]
            : [GridItem(.flexible()), GridItem(.flexible())]
    }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                header
                currentState
                PlantSensorSection(series: store.series)
                PlantHealthSection(plant: store.plant)
                EvidenceWorkflowSection(
                    plant: store.plant,
                    photos: store.timeline.photos,
                    observations: store.timeline.observations,
                    record: store.recordEntry
                )

                if !store.plant.status.isRetired {
                    actions
                } else {
                    referenceActions
                }

                PlantFactsCard(plant: store.plant)

                if let toxicity = store.toxicity {
                    PlantToxicitySection(plant: store.plant, toxicity: toxicity)
                }

                if let actionError {
                    actionErrorCard(actionError)
                }
                if store.plant.status == .dead {
                    postmortemCard
                }

                if let error = store.error {
                    chapterErrorCard(error)
                }

                SectionHeading("History", detail: "Photos, care, and the changes Planty noticed over time.")

                if store.hasStory {
                    ForEach(store.chapters) { chapter in
                        ChapterRow(chapter: chapter, plant: store.plant) { photo in
                            deletingPhoto = photo
                        }
                    }
                } else if store.hasLoaded {
                    emptyStory
                }

                WhyPlantyThinksThis(series: store.series, verdict: store.verdict)
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 16)
            .frame(maxWidth: .infinity, alignment: .leading)
            .plantyReadableContent()
        }
        .plantyPage()
        .navigationTitle(store.plant.commonName)
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await refresh() }
        .task { await refresh() }
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Menu {
                    Button {
                        isEditing = true
                    } label: {
                        Label("Edit plant", systemImage: "pencil")
                    }
                    Button {
                        isEditingToxicity = true
                    } label: {
                        Label("Edit toxicity", systemImage: "cross.case")
                    }
                    Button {
                        isShowingLabel = true
                    } label: {
                        Label("Print QR label", systemImage: "qrcode")
                    }
                    if !store.plant.status.isRetired {
                        Button {
                            isConfirmingArchive = true
                        } label: {
                            Label("Archive plant…", systemImage: "archivebox")
                        }
                        Button(role: .destructive) {
                            isConfirmingDeath = true
                        } label: {
                            Label("It died…", systemImage: "xmark.seal")
                        }
                    } else {
                        Button {
                            Task { await restorePlant() }
                        } label: {
                            Label("Restore plant", systemImage: "arrow.uturn.backward.circle")
                        }
                    }
                } label: {
                    Image(systemName: "ellipsis.circle")
                }
                .accessibilityLabel("Plant actions")
            }
        }
        .sheet(isPresented: $isEditing) {
            EditPlantSheet(plant: store.plant, choices: session.choices) { patch in
                let failure = await store.saveEdits(patch)
                if failure == nil { session.library.apply(store.plant) }
                return failure
            } setSheltered: { indoors in
                await store.setSheltered(indoors)
            }
        }
        .sheet(isPresented: $isShowingLabel) {
            PlantLabelSheet(plant: store.plant)
        }
        .sheet(isPresented: $isEditingToxicity) {
            ToxicityEditSheet(plant: store.plant, toxicity: store.toxicity) { toxicity in
                var patch = PlantPatch()
                patch.toxicity = toxicity
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
        .sheet(isPresented: $isManagingAirflow) {
            AirflowControlSheet(plant: store.plant) {
                await store.load()
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
            Text("The story and photos stay. You can restore the plant later if this was a mistake.")
        }
        .confirmationDialog(
            "Archive \(store.plant.commonName)?",
            isPresented: $isConfirmingArchive,
            titleVisibility: .visible
        ) {
            Button("Archive") { Task { await archivePlant() } }
            Button("Keep active", role: .cancel) {}
        } message: {
            Text(
                "This removes the plant from the active garden without deleting its story or photos. "
                    + "You can restore it later."
            )
        }
        .confirmationDialog(
            "Delete this photo?",
            isPresented: .init(get: { deletingPhoto != nil }, set: { if !$0 { deletingPhoto = nil } }),
            titleVisibility: .visible
        ) {
            Button("Delete photo", role: .destructive) {
                guard let photo = deletingPhoto else { return }
                deletingPhoto = nil
                Task { actionError = await store.deletePhoto(photo) }
            }
            Button("Keep photo", role: .cancel) { deletingPhoto = nil }
        } message: {
            Text("The image will be removed from Planty and object storage. This cannot be undone.")
        }
    }

    private func recordDeath() async {
        actionError = await store.archive(as: .dead)
        if actionError == nil { session.library.apply(store.plant) }
    }

    private func archivePlant() async {
        actionError = await store.archive(as: .gone)
        if actionError == nil { session.library.apply(store.plant) }
    }

    private func restorePlant() async {
        actionError = await store.restore()
        if actionError == nil { session.library.apply(store.plant) }
    }

    private func refresh() async {
        async let story: Void = store.load()
        async let health: Void = session.health.load(store.plant)
        async let actuators: Void = session.actuators.load()
        _ = await (story, health, actuators)
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: 10) {
            PlantPhotoView(
                plant: store.plant,
                photo: store.latestPhoto,
                height: 250
            )

            HStack(alignment: .top, spacing: 10) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(store.plant.commonName)
                        .font(.largeTitle.weight(.bold))
                    if let species = store.plant.displaySpecies {
                        Text(species)
                            .font(.subheadline)
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                    if !store.plant.location.isEmpty {
                        Label(store.plant.location, systemImage: "mappin.and.ellipse")
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                }
                Spacer(minLength: 4)
                OwnershipBadge(plant: store.plant)
            }

            if comparison.isPossible {
                NavigationLink {
                    PhotoComparisonScreen(plant: store.plant, comparison: comparison)
                } label: {
                    Label("Compare photos", systemImage: "rectangle.split.2x1")
                        .font(.subheadline.weight(.semibold))
                }
                .foregroundStyle(PlantyColor.green)
                .frame(minHeight: 44)
            }
        }
    }

    private var currentState: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                StatusPill(state: store.careState)
                Spacer()
                Text(store.lastComparedLine)
                    .font(.caption)
                    .foregroundStyle(store.freshness.isFresh ? PlantyColor.green : PlantyColor.orange)
                    .multilineTextAlignment(.trailing)
            }
            Text(headline)
                .font(.title2.weight(.bold))
            Text(store.verdict?.reasoning ?? store.careState.sentence)
                .font(.body)
                .foregroundStyle(PlantyColor.secondaryText)
            if store.hasPhotos {
                Button {
                    Task { actionError = await store.assessNow() }
                } label: {
                    HStack {
                        if store.isAssessing {
                            ProgressView()
                        } else {
                            Image(systemName: "sparkles")
                        }
                        Text(store.isAssessing ? "Analyzing latest photo…" : "Analyze latest photo again")
                    }
                    .frame(maxWidth: .infinity)
                }
                .buttonStyle(SecondaryButtonStyle())
                .disabled(store.isAssessing)
                .accessibilityHint("Runs a fresh Planty assessment now using the newest photo and current care record")
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: store.careState.color.opacity(0.22))
    }

    private var actions: some View {
        VStack(alignment: .leading, spacing: 12) {
            SectionHeading("Actions")
            LazyVGrid(columns: actionColumns, spacing: 10) {
                Button {
                    session.beginCapture(for: store.plant)
                } label: {
                    ActionFace("Take photo", icon: "camera.fill")
                }
                .buttonStyle(SecondaryButtonStyle())
                .accessibilityLabel("Take a photo of \(store.plant.commonName)")

                NavigationLink {
                    PlantChatsScreen(store: session.chatsStore(for: store.plant))
                } label: {
                    ActionFace("Planty chats", icon: "bubble.left.and.text.bubble.right.fill")
                }
                .buttonStyle(SecondaryButtonStyle())

                Button {
                    isLoggingCare = true
                } label: {
                    ActionFace("Log care", icon: "checkmark.circle.fill")
                }
                .buttonStyle(SecondaryButtonStyle())
                .accessibilityLabel("Log care for \(store.plant.commonName)")

                if !plantActuators.isEmpty {
                    Button {
                        isManagingAirflow = true
                    } label: {
                        ActionFace(
                            activePlantActuator == nil ? "Airflow" : "Fan running",
                            icon: "fan.fill",
                            detail: activePlantActuator == nil ? "Choose a time" : "Tap to view or stop"
                        )
                    }
                    .buttonStyle(SecondaryButtonStyle())
                    .accessibilityLabel("Control airflow for \(store.plant.commonName)")
                }

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

                if isEdible {
                    Button {
                        isHarvesting = true
                    } label: {
                        ActionFace("Harvest", icon: "basket.fill")
                    }
                    .buttonStyle(SecondaryButtonStyle())
                    .accessibilityLabel("Log a harvest from \(store.plant.commonName)")
                }
            }
        }
    }

    private var plantActuators: [Actuator] {
        session.actuators.registered.assigned(to: store.plant.id)
    }

    private var activePlantActuator: Actuator? {
        plantActuators.first { session.actuators.leases[$0.id]?.isActive == true }
    }

    private var referenceActions: some View {
        VStack(alignment: .leading, spacing: 12) {
            SectionHeading("Reference")
            LazyVGrid(columns: actionColumns, spacing: 10) {
                NavigationLink {
                    PlantChatsScreen(store: session.chatsStore(for: store.plant))
                } label: {
                    ActionFace("Planty chats", icon: "bubble.left.and.text.bubble.right.fill")
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
    }

    private var isEdible: Bool {
        store.plant.domain == .edibleIndoor || store.plant.domain == .edibleOutdoor
    }

    private var comparison: PhotoComparison {
        PhotoComparison(store.timeline.photos)
    }

    private var headline: String {
        switch store.careState {
        case .allGood: "Nothing to do right now"
        case .watch: "Keep an eye on this"
        case .needsCare: "One thing needs doing"
        case .urgent: "This needs attention now"
        case .unknown: "Planty cannot say yet"
        }
    }

    private var emptyStory: some View {
        StateMessage(
            title: store.hasPhotos ? "No changes worth calling out yet" : "Add the first photo",
            message: store.hasPhotos
                ? "Photos, care actions, and useful changes will collect here over time."
                : "Sensors can describe the pot and room. A photo adds the part they cannot see.",
            accent: PlantyColor.pink,
            icon: "camera.fill"
        ) {
            Button("Take a photo") { session.beginCapture(for: store.plant) }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.pink))
        }
    }

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
                        .foregroundStyle(PlantyColor.orange)
                        .padding(.top, 4)
                    Text(lesson)
                        .font(.title3.weight(.bold))
                }
            } else if store.isAskingPostmortem {
                HStack(alignment: .top, spacing: 12) {
                    ProgressView()
                    Text("Planty is reading the whole story back before it answers.")
                        .font(.subheadline)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
                .accessibilityElement(children: .combine)
            } else {
                Text("\(store.plant.commonName) is recorded as dead. The story stays available.")
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
        .plantyCard(border: PlantyColor.purple.opacity(0.22))
    }

    private func actionErrorCard(_ failure: PlantyError) -> some View {
        StateMessage(
            title: "That change was not saved",
            message: failure.errorDescription ?? "The service could not be reached.",
            accent: PlantyColor.orange,
            icon: "exclamationmark.triangle.fill"
        ) {
            Button("Dismiss") { actionError = nil }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
        }
    }

    private func chapterErrorCard(_ error: PlantyError) -> some View {
        StateMessage(
            title: "The newest history is missing",
            message: "Earlier photos and notes are still available. Planty could not load the latest events.",
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
    let deletePhoto: (Photo) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Eyebrow(text: chapter.dateLabel, color: PlantyColor.secondaryText)
            Text(chapter.title)
                .font(.headline)

            if let photo = chapter.photo {
                PlantPhotoView(plant: plant, photo: photo, height: 200)
                Button(role: .destructive) {
                    deletePhoto(photo)
                } label: {
                    Label("Delete photo", systemImage: "trash")
                        .frame(minHeight: 44)
                }
                .font(.subheadline.weight(.semibold))
            }

            Text(chapter.narrative)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)

            ForEach(chapter.events) { event in
                if event.photo == nil {
                    HStack(spacing: 6) {
                        Label(event.title, systemImage: event.symbol)
                            .font(.footnote)
                            .foregroundStyle(event.careState?.color ?? PlantyColor.secondaryText)
                        Text(event.timeLabel)
                            .font(.caption2)
                            .foregroundStyle(PlantyColor.secondaryText)
                        Spacer(minLength: 0)
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard()
        .accessibilityElement(children: .contain)
    }
}
