import Foundation
import Observation

/// The library: every plant, grouped by who owns it and filtered by search.
@Observable
@MainActor
final class PlantsStore {
    private(set) var plants: [Plant] = []
    private(set) var error: PlantyError?
    private(set) var isLoading = false
    private(set) var hasLoaded = false

    var searchText = ""
    var showArchived = false

    var isConfigured: Bool
    private var api: any PlantyAPI
    private var loadGeneration = 0

    init(api: any PlantyAPI, isConfigured: Bool) {
        self.api = api
        self.isConfigured = isConfigured
    }

    func replace(api: any PlantyAPI, isConfigured: Bool) {
        loadGeneration += 1
        self.api = api
        self.isConfigured = isConfigured
        plants = []
        error = nil
        isLoading = false
        hasLoaded = false
    }

    func load() async {
        guard isConfigured else { return }
        loadGeneration += 1
        let generation = loadGeneration
        let client = api
        isLoading = true
        error = nil
        defer {
            if generation == loadGeneration { isLoading = false }
        }

        do {
            var filter = PlantFilter.live
            filter.includeArchived = showArchived
            let loaded = try await client.plants(filter: filter)
            guard generation == loadGeneration else { return }
            plants = loaded
            hasLoaded = true
        } catch {
            guard generation == loadGeneration else { return }
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
    }

    /// The failure goes back to the sheet that asked, not into `error`: that
    /// slot repaints the whole library as a loading problem, and a create that
    /// did not land is the sheet's news to break while the typing still exists.
    func create(_ draft: NewPlant) async -> PlantyError? {
        do {
            let created = try await api.createPlant(draft)
            plants.append(created)
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    /// A screen that changed one plant pushes the fresh copy in, so the list is
    /// right without another round trip. Retired plants leave the list the same
    /// way GET /v1/plants would drop them.
    func apply(_ updated: Plant) {
        guard let index = plants.firstIndex(where: { $0.id == updated.id }) else { return }
        if updated.status.isRetired && !showArchived {
            plants.remove(at: index)
        } else {
            plants[index] = updated.keepingPhoto(from: plants[index])
        }
    }

    /// Search covers name, species, owner and room, because a beginner rarely
    /// remembers which of those they used.
    var matches: [Plant] {
        let needle = searchText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !needle.isEmpty else { return plants }
        return plants.filter { plant in
            [
                plant.commonName,
                plant.botanicalName ?? "",
                plant.variety ?? "",
                plant.location,
                plant.steward
            ]
            .contains { $0.localizedCaseInsensitiveContains(needle) }
        }
    }

    /// Friend groups first, one per owner, then mine. Sorting is the entire
    /// privilege ownership buys; it never changes how a row is coloured.
    var groups: [PlantGroup] {
        let sorted = matches.sorted {
            $0.commonName.localizedCompare($1.commonName) == .orderedAscending
        }
        let friends = Dictionary(grouping: sorted.filter(\.isFriends), by: \.steward)
        let mine = sorted.filter { !$0.isFriends }

        var groups = friends.keys.sorted().map { owner in
            PlantGroup(title: "\(owner)'s plants", isFriendOwned: true, plants: friends[owner] ?? [])
        }
        if !mine.isEmpty {
            groups.append(PlantGroup(title: "Mine", isFriendOwned: false, plants: mine))
        }
        return groups
    }

    var isEmptyLibrary: Bool { hasLoaded && plants.isEmpty }
    var hasNoMatches: Bool { hasLoaded && !plants.isEmpty && matches.isEmpty }

    func clearError() { error = nil }
}

struct PlantGroup: Sendable, Hashable, Identifiable {
    let title: String
    let isFriendOwned: Bool
    let plants: [Plant]

    var id: String { title }
}
