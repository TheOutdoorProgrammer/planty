import Foundation
import Observation

@Observable
@MainActor
final class ManagedChoicesStore {
    private(set) var catalog: ManagedChoices = .empty
    private(set) var error: PlantyError?
    private(set) var hasLoaded = false

    private var api: any PlantyAPI
    private var isConfigured: Bool
    private var loadGeneration = 0

    init(api: any PlantyAPI, isConfigured: Bool) {
        self.api = api
        self.isConfigured = isConfigured
    }

    func replace(api: any PlantyAPI, isConfigured: Bool) {
        loadGeneration += 1
        self.api = api
        self.isConfigured = isConfigured
        catalog = .empty
        error = nil
        hasLoaded = false
    }

    func loadIfNeeded() async {
        guard !hasLoaded else { return }
        await load()
    }

    func load() async {
        guard isConfigured else {
            catalog = .empty
            error = nil
            hasLoaded = true
            return
        }
        let generation = loadGeneration
        do {
            let loaded = try await api.managedChoices()
            guard generation == loadGeneration else { return }
            catalog = loaded
            error = nil
            hasLoaded = true
        } catch {
            guard !PlantyError.isCancellation(error), generation == loadGeneration else { return }
            self.error = PlantyError.from(error)
            hasLoaded = true
        }
    }
}
