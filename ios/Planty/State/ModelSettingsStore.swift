import Foundation
import Observation

@Observable
@MainActor
final class ModelSettingsStore {
    private(set) var models: [AIModel] = []
    private(set) var assignments: [JobAssignment] = []
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
        models = []
        assignments = []
        error = nil
        hasLoaded = false
    }

    func loadIfNeeded() async {
        guard !hasLoaded else { return }
        await load()
    }

    func load() async {
        guard isConfigured else {
            models = []
            assignments = []
            error = nil
            hasLoaded = true
            return
        }
        let generation = loadGeneration
        do {
            let catalog = try await api.aiModels()
            let assigned = try await api.jobAssignments()
            guard generation == loadGeneration else { return }
            models = catalog
            assignments = assigned
            error = nil
            hasLoaded = true
        } catch {
            guard !PlantyError.isCancellation(error), generation == loadGeneration else { return }
            self.error = PlantyError.from(error)
            hasLoaded = true
        }
    }

    /// Only the models the server says may do this job. The phone never decides
    /// capability itself, so a model that cannot see is not merely discouraged
    /// for identification, it is absent.
    func choices(for job: AIJob) -> [AIModel] {
        models.filter { $0.can(job) }
    }

    func assignment(for job: AIJob) -> JobAssignment? {
        assignments.first { $0.job == job }
    }

    func model(ref: String?) -> AIModel? {
        guard let ref else { return nil }
        return models.first { $0.ref == ref }
    }

    func assign(_ model: AIModel, to job: AIJob) async -> PlantyError? {
        await apply(job: job) {
            try await self.api.assign(job: job, provider: model.provider, model: model.modelID)
        }
    }

    func useDefault(for job: AIJob) async -> PlantyError? {
        await apply(job: job) { try await self.api.clearAssignment(job: job) }
    }

    private func apply(job: AIJob, _ write: @escaping () async throws -> JobAssignment) async -> PlantyError? {
        do {
            let saved = try await write()
            if let index = assignments.firstIndex(where: { $0.job == job }) {
                assignments[index] = saved
            } else {
                assignments.append(saved)
            }
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
