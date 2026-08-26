import SwiftUI

/// A searchable visual library. Camera is a visible action on every plant;
/// users no longer need to discover a hidden swipe gesture to take a photo.
struct PlantsLibraryScreen: View {
    @Environment(AppSession.self) private var session
    @State private var isAdding = false
    @State private var route: [Plant] = []

    private var store: PlantsStore { session.library }

    var body: some View {
        @Bindable var store = store

        NavigationStack(path: $route) {
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
                    library
                }
            }
            .plantyPage()
            .navigationTitle("Plants")
            .searchable(text: $store.searchText, prompt: "Name, species, owner or room")
            .refreshable { await loadWithStatuses() }
            .task { await loadWithStatuses() }
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button(store.showArchived ? "Active only" : "All plants") {
                        store.showArchived.toggle()
                        Task { await loadWithStatuses() }
                    }
                    .accessibilityLabel(store.showArchived ? "Hide archived plants" : "Include archived plants")
                }
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
            .onChange(of: session.pendingPlantSlug) { _, slug in
                guard let slug else { return }
                Task { await openPlant(slug: slug) }
            }
        }
    }

    private func openPlant(slug: String) async {
        if store.plants.first(where: { $0.slug == slug }) == nil {
            store.showArchived = true
            await store.load()
        }
        if let plant = store.plants.first(where: { $0.slug == slug }) {
            route = [plant]
        }
        session.pendingPlantSlug = nil
    }

    private var library: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 22) {
                if let error = store.error {
                    partialError(error)
                }

                ForEach(store.groups) { group in
                    VStack(alignment: .leading, spacing: 10) {
                        HStack {
                            Text(group.title)
                                .font(.headline)
                                .foregroundStyle(group.isFriendOwned ? PlantyColor.purple : PlantyColor.foreground)
                            Spacer()
                            Text(group.plants.count.formatted())
                                .font(.caption.weight(.semibold))
                                .foregroundStyle(PlantyColor.secondaryText)
                        }

                        ForEach(group.plants) { plant in
                            PlantLibraryRow(plant: plant, state: state(for: plant)) {
                                session.beginCapture(for: plant)
                            }
                        }
                    }
                }
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 16)
            .plantyReadableContent()
        }
    }

    private func partialError(_ error: PlantyError) -> some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: "wifi.exclamationmark")
                .foregroundStyle(PlantyColor.orange)
            VStack(alignment: .leading, spacing: 3) {
                Text("This list may be out of date")
                    .font(.subheadline.weight(.semibold))
                if let detail = error.errorDescription {
                    Text(detail)
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            }
            Spacer(minLength: 4)
            Button("Retry") { Task { await store.load() } }
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(PlantyColor.orange)
                .frame(minHeight: 44)
        }
        .plantyCard(border: PlantyColor.orange.opacity(0.2), padding: 14)
    }

    private func loadWithStatuses() async {
        await store.load()
        await session.health.load(store.plants)
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
                .padding(.horizontal, 16)
                .padding(.vertical, 20)
        }
    }

    private var emptyCard: some View {
        StateMessage(
            title: "Add your first plant",
            message: "Start with a photo. Name it now, let Planty suggest one, or keep the nickname you actually use.",
            accent: PlantyColor.green,
            icon: "leaf.fill"
        ) {
            Button("Add a plant") { isAdding = true }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.green))
        }
    }

    private var noResultsCard: some View {
        StateMessage(
            title: "No matches",
            message: "Try a room, owner, species, or a different name.",
            accent: PlantyColor.cyan,
            icon: "magnifyingglass"
        ) {
            Button("Add a new plant") { isAdding = true }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.cyan))
        }
    }

    private func errorCard(_ error: PlantyError) -> some View {
        StateMessage(
            title: "The plant list did not load",
            message: "Saved photos are still safe. Try loading the library again.",
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

struct PlantLibraryRow: View {
    let plant: Plant
    let state: CareState
    let takePhoto: () -> Void

    @Environment(AppSession.self) private var session

    var body: some View {
        ZStack(alignment: .trailing) {
            NavigationLink(value: plant) {
                HStack(spacing: 12) {
                    PlantPhotoView(plant: plant, height: 72, opensFullScreen: false)
                        .frame(width: 72)

                    VStack(alignment: .leading, spacing: 5) {
                        Text(plant.commonName)
                            .font(.headline)
                            .foregroundStyle(PlantyColor.foreground)
                        StatusPill(state: state)
                        PlantHealthBar(event: session.health.current(for: plant), compact: true)
                        Text(metadata)
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                            .lineLimit(2)
                    }
                    Spacer(minLength: 2)
                }
                .padding(.trailing, 56)
                .frame(maxWidth: .infinity, alignment: .leading)
                .contentShape(Rectangle())
                .plantyCard(padding: 12)
            }
            .buttonStyle(.plain)

            Button(action: takePhoto) {
                Image(systemName: "camera.fill")
                    .font(.body.weight(.semibold))
                    .foregroundStyle(PlantyColor.pink)
                    .frame(width: 44, height: 44)
                    .background(PlantyColor.pink.opacity(0.1), in: Circle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Take a photo of \(plant.commonName)")
            .padding(.trailing, 12)
        }
    }

    private var metadata: String {
        [plant.location, plant.isFriends ? plant.ownershipLabel : nil, plant.displaySpecies]
            .compactMap { $0 }
            .filter { !$0.isEmpty }
            .joined(separator: " · ")
    }
}
