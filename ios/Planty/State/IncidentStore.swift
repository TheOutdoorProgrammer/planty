import Foundation
import Observation

@Observable
@MainActor
final class IncidentStore {
    private(set) var incidents: [GardenIncident] = []
    private(set) var error: PlantyError?
    private(set) var hasLoaded = false
    private(set) var changing: Set<UUID> = []

    private var api: any PlantyAPI
    private var isConfigured: Bool
    private var generation = 0

    init(api: any PlantyAPI, isConfigured: Bool) {
        self.api = api
        self.isConfigured = isConfigured
    }

    func replace(api: any PlantyAPI, isConfigured: Bool) {
        generation += 1
        self.api = api
        self.isConfigured = isConfigured
        incidents = []
        error = nil
        hasLoaded = false
        changing = []
    }

    var unresolved: [GardenIncident] {
        incidents.filter { $0.status == .open || $0.status == .acknowledged }
            .sorted { $0.lastSeenAt > $1.lastSeenAt }
    }

    func load() async {
        guard isConfigured else { hasLoaded = true; incidents = []; return }
        let startedGeneration = generation
        do {
            let loaded = try await api.incidentList(status: nil)
            guard startedGeneration == generation else { return }
            incidents = loaded.sorted { $0.lastSeenAt > $1.lastSeenAt }
            error = nil
            hasLoaded = true
        } catch {
            guard startedGeneration == generation, !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
            hasLoaded = true
        }
    }

    func acknowledge(_ incident: GardenIncident) async -> PlantyError? {
        await change(incident) { try await self.api.acknowledgeIncident(id: incident.id, actor: "owner") }
    }

    func resolve(_ incident: GardenIncident, outcome: IncidentResolution, conclusion: String) async -> PlantyError? {
        let conclusion = conclusion.cleaned
        guard !conclusion.isEmpty else { return .transport("Record what the evidence showed.") }
        return await change(incident) {
            try await self.api.resolveIncident(
                id: incident.id,
                request: IncidentResolutionRequest(outcome: outcome, actor: "owner", conclusion: conclusion)
            )
        }
    }

    private func change(
        _ incident: GardenIncident,
        operation: @escaping () async throws -> GardenIncident
    ) async -> PlantyError? {
        guard !changing.contains(incident.id) else { return nil }
        changing.insert(incident.id)
        defer { changing.remove(incident.id) }
        do {
            let saved = try await operation()
            incidents.removeAll { $0.id == saved.id }
            incidents.append(saved)
            incidents.sort { $0.lastSeenAt > $1.lastSeenAt }
            error = nil
            return nil
        } catch {
            guard !PlantyError.isCancellation(error) else { return nil }
            let failure = PlantyError.from(error)
            self.error = failure
            return failure
        }
    }
}
