import SwiftUI

/// A searchable visual list grouped by owner. No sensor gauges anywhere.
struct PlantsLibraryScreen: View {
    @Environment(AppSession.self) private var session
    @State private var isAdding = false

    private var store: PlantsStore { session.library }

    var body: some View {
        @Bindable var store = store

        NavigationStack {
            Group {
                if !store.isConfigured {
                    scrollWrapped {
                        UnconfiguredCard { session.isShowingSettings = true }
                    }
                } else if let error = store.error, store.plants.isEmpty {
                    scrollWrapped { errorCard(error) }
                } else if store.isEmptyLibrary {
                    scrollWrapped { emptyCard }
                } else if store.hasNoMatches {
                    scrollWrapped { noResultsCard }
                } else {
                    list
                }
            }
            .plantyPage()
            .navigationTitle("Plants")
            .searchable(text: $store.searchText, prompt: "Name, species, owner or room")
            .refreshable { await store.load() }
            .task { await loadWithStatuses() }
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button { isAdding = true } label: {
                        Image(systemName: "plus")
                    }
                    .accessibilityLabel("Add a plant")
                }
            }
            .sheet(isPresented: $isAdding) {
                AddPlantSheet(choices: session.choices) { draft in
                    await store.create(draft)
                }
            }
            .navigationDestination(for: Plant.self) { plant in
                PlantStoryScreen(store: session.storyStore(for: plant))
            }
        }
    }

    private var list: some View {
        List {
            // A refresh that failed with plants on screen used to say nothing,
            // which reads as "everything loaded fine".
            if let error = store.error {
                Section {
                    VStack(alignment: .leading, spacing: 8) {
                        Label("This list may be out of date.", systemImage: "wifi.exclamationmark")
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(PlantyColor.orange)
                        if let detail = error.errorDescription {
                            Text(detail)
                                .font(.footnote)
                                .foregroundStyle(PlantyColor.secondaryText)
                        }
                        Button("Try again") { Task { await store.load() } }
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(PlantyColor.orange)
                            .frame(minHeight: 44)
                            .accessibilityLabel("Reload the plant list")
                    }
                    .listRowBackground(PlantyColor.orange.opacity(0.12))
                }
            }
            ForEach(store.groups) { group in
                Section {
                    ForEach(group.plants) { plant in
                        NavigationLink(value: plant) {
                            PlantLibraryRow(plant: plant, state: state(for: plant))
                        }
                        .listRowBackground(PlantyColor.surface)
                        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
                            Button {
                                session.beginCapture(for: plant)
                            } label: {
                                Label("Take photo", systemImage: "camera.fill")
                            }
                            .tint(PlantyColor.pink)
                        }
                    }
                } header: {
                    Text(group.title)
                        .font(.headline)
                        .foregroundStyle(
                            group.isFriendOwned ? PlantyColor.purple : PlantyColor.secondaryText
                        )
                }
            }
        }
        .listStyle(.insetGrouped)
        .scrollContentBackground(.hidden)
    }

    /// Without the digest every row honestly reads Unknown, so fetch it even
    /// when the user opened this tab first.
    private func loadWithStatuses() async {
        await store.load()
        if session.today.digest == nil {
            await session.today.load()
        }
    }

    private func state(for plant: Plant) -> CareState {
        LibraryStatus.state(
            for: plant,
            digest: session.today.digest,
            now: Date(),
            knownPlantCount: store.plants.count
        )
    }

    private func scrollWrapped<Content: View>(@ViewBuilder _ content: () -> Content) -> some View {
        ScrollView {
            content()
                .padding(.horizontal, 20)
                .padding(.vertical, 24)
        }
    }

    private var emptyCard: some View {
        VStack(spacing: 18) {
            PlantyMascot(wobbles: false)
            StateMessage(
                title: "No plants yet.",
                message: """
                    Start with a photo. You can name the plant now, let Planty \
                    suggest one, or just call it "the dramatic one."
                    """,
                accent: PlantyColor.pink,
                icon: "leaf.fill"
            ) {
                Button("Add the first plant") { isAdding = true }
                    .buttonStyle(PrimaryButtonStyle())
            }
        }
    }

    private var noResultsCard: some View {
        StateMessage(
            title: "No plant by that name.",
            message: "Try a room or species, or add this plant with a photo.",
            accent: PlantyColor.cyan,
            icon: "magnifyingglass"
        ) {
            Button("Add a new plant") { isAdding = true }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.cyan))
        }
    }

    private func errorCard(_ error: PlantyError) -> some View {
        StateMessage(
            title: "The plant list did not load.",
            message: "Planty will keep trying. Today's saved photos are still safe.",
            accent: PlantyColor.orange,
            icon: "arrow.trianglehead.clockwise"
        ) {
            if let detail = error.errorDescription {
                Text(detail)
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            Button("Try again") { Task { await store.load() } }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
        }
    }
}

/// Status words are more prominent than the species, and there is no gauge.
struct PlantLibraryRow: View {
    let plant: Plant
    let state: CareState

    var body: some View {
        HStack(spacing: 14) {
            // A tap on this row means "open the plant", so the thumbnail must
            // not steal it to show one picture full screen.
            PlantPhotoView(plant: plant, height: 64, opensFullScreen: false)
                .frame(width: 64)

            VStack(alignment: .leading, spacing: 5) {
                Text(plant.commonName)
                    .font(.headline)
                    .foregroundStyle(PlantyColor.foreground)
                StatusPill(state: state)
                if let species = plant.displaySpecies {
                    Text(species)
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
                Text(plant.location)
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
                OwnershipBadge(plant: plant)
            }

            Spacer(minLength: 4)
        }
        .padding(.vertical, 6)
        .accessibilityElement(children: .combine)
    }
}
