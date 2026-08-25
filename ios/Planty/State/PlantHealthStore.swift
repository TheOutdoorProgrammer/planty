import Foundation
import Observation

/// Health is an append-only evidence stream shared by the library and story.
/// The absence of a current event remains unknown; it never becomes zero.
@Observable
@MainActor
final class PlantHealthStore {
    private(set) var records: [String: PlantHealthResponse] = [:]
    private(set) var loaded: Set<String> = []
    private(set) var loading: Set<String> = []
    private(set) var errors: [String: PlantyError] = [:]

    private var api: any PlantyAPI
    private var generation = 0

    init(api: any PlantyAPI) {
        self.api = api
    }

    func replace(api: any PlantyAPI) {
        generation += 1
        self.api = api
        records = [:]
        loaded = []
        loading = []
        errors = [:]
    }

    func response(for plant: Plant) -> PlantHealthResponse? { records[plant.slug] }
    func current(for plant: Plant) -> HealthEvent? { records[plant.slug]?.current }
    func history(for plant: Plant) -> [HealthEvent] { records[plant.slug]?.events ?? [] }
    func error(for plant: Plant) -> PlantyError? { errors[plant.slug] }
    func isLoading(_ plant: Plant) -> Bool { loading.contains(plant.slug) }
    func hasLoaded(_ plant: Plant) -> Bool { loaded.contains(plant.slug) }

    func load(_ plant: Plant) async {
        let slug = plant.slug
        guard !loading.contains(slug) else { return }
        let startedGeneration = generation
        let client = api
        loading.insert(slug)
        defer {
            if startedGeneration == generation { loading.remove(slug) }
        }

        do {
            var response = try await client.plantHealth(slug: slug)
            guard startedGeneration == generation else { return }
            response.events.sort {
                if $0.createdAt != $1.createdAt { return $0.createdAt > $1.createdAt }
                return $0.id.uuidString > $1.id.uuidString
            }
            response.current = response.events.first ?? response.current
            records[slug] = response
            loaded.insert(slug)
            errors.removeValue(forKey: slug)
        } catch {
            guard startedGeneration == generation,
                  !PlantyError.isCancellation(error)
            else { return }
            errors[slug] = PlantyError.from(error)
            loaded.insert(slug)
        }
    }

    func load(_ plants: [Plant]) async {
        await withTaskGroup(of: Void.self) { group in
            for plant in plants {
                group.addTask { await self.load(plant) }
            }
        }
    }

    @discardableResult
    func save(_ change: NewHealthChange, for plant: Plant) async -> PlantyError? {
        let slug = plant.slug
        do {
            let event = try await api.addHealthEvent(slug: slug, change: change)
            var response = records[slug] ?? .unknown
            response.events.removeAll { $0.id == event.id }
            response.events.insert(event, at: 0)
            response.current = event
            response.count = response.events.count
            records[slug] = response
            loaded.insert(slug)
            errors.removeValue(forKey: slug)
            return nil
        } catch {
            guard !PlantyError.isCancellation(error) else { return nil }
            let failure = PlantyError.from(error)
            errors[slug] = failure
            return failure
        }
    }
}
