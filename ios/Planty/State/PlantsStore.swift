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

    var isConfigured: Bool
    private var api: any PlantyAPI

    init(api: any PlantyAPI, isConfigured: Bool) {
        self.api = api
        self.isConfigured = isConfigured
    }

    func replace(api: any PlantyAPI, isConfigured: Bool) {
        self.api = api
        self.isConfigured = isConfigured
        plants = []
        error = nil
        hasLoaded = false
    }

    func load() async {
        guard isConfigured else { return }
        isLoading = true
        error = nil
        defer { isLoading = false }

        do {
            plants = try await api.plants(filter: .live)
            hasLoaded = true
        } catch {
            self.error = PlantyError.from(error)
        }
    }

    func create(_ draft: NewPlant) async -> Plant? {
        do {
            let created = try await api.createPlant(draft)
            plants.append(created)
            return created
        } catch {
            self.error = PlantyError.from(error)
            return nil
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
