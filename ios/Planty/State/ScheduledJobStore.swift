import Foundation
import Observation

@Observable
@MainActor
final class ScheduledJobStore {
    private(set) var jobs: [ScheduledJob] = []
    private(set) var error: PlantyError?
    private(set) var hasLoaded = false
    private(set) var launching: Set<ScheduledJobID> = []

    private var api: any PlantyAPI
    private var isConfigured: Bool
    private var generation = 0
    private let pollInterval: Duration

    init(
        api: any PlantyAPI,
        isConfigured: Bool,
        pollInterval: Duration = .seconds(2)
    ) {
        self.api = api
        self.isConfigured = isConfigured
        self.pollInterval = pollInterval
    }

    func replace(api: any PlantyAPI, isConfigured: Bool) {
        generation += 1
        self.api = api
        self.isConfigured = isConfigured
        jobs = []
        error = nil
        hasLoaded = false
        launching = []
    }

    func loadIfNeeded() async {
        guard !hasLoaded else { return }
        await load()
    }

    func load() async {
        guard isConfigured else {
            jobs = []
            error = nil
            hasLoaded = true
            return
        }
        let currentGeneration = generation
        do {
            let loaded = try await api.scheduledJobs()
            guard currentGeneration == generation else { return }
            jobs = loaded
            error = nil
            hasLoaded = true
        } catch {
            guard currentGeneration == generation, !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
            hasLoaded = true
        }
    }

    func job(_ id: ScheduledJobID) -> ScheduledJob? {
        jobs.first { $0.id == id }
    }

    func isRunning(_ id: ScheduledJobID) -> Bool {
        launching.contains(id) || job(id)?.latestRun?.state.isActive == true
    }

    /// Starts the exact code-owned CronJob and follows its durable Kubernetes
    /// run until completion. Leaving the screen does not stop the Job; another
    /// tap finds and follows the same active run instead of creating a second.
    @discardableResult
    func run(_ id: ScheduledJobID) async -> PlantyError? {
        guard isConfigured, id != .unknown, !launching.contains(id) else { return nil }
        let currentGeneration = generation
        launching.insert(id)
        error = nil
        defer { launching.remove(id) }

        do {
            let started = try await api.runScheduledJob(id)
            guard currentGeneration == generation else { return nil }
            apply(started, to: id)
            if let failure = terminalFailure(started) {
                throw failure
            }
            if started.state == .succeeded {
                return nil
            }

            while !Task.isCancelled {
                try await Task.sleep(for: pollInterval)
                let loaded = try await api.scheduledJobs()
                guard currentGeneration == generation else { return nil }
                jobs = loaded
                hasLoaded = true

                guard let latest = job(id)?.latestRun else { continue }
                if latest.id != started.id, latest.state.isActive { continue }
                if let failure = terminalFailure(latest) {
                    throw failure
                }
                if latest.state == .succeeded {
                    error = nil
                    return nil
                }
            }
            return nil
        } catch {
            guard !PlantyError.isCancellation(error), currentGeneration == generation else { return nil }
            let failure = PlantyError.from(error)
            self.error = failure
            return failure
        }
    }

    func clearError() { error = nil }

    private func apply(_ run: ScheduledJobRun, to id: ScheduledJobID) {
        guard let index = jobs.firstIndex(where: { $0.id == id }) else { return }
        jobs[index].latestRun = run
    }

    private func terminalFailure(_ run: ScheduledJobRun) -> PlantyError? {
        guard run.state == .failed else { return nil }
        return .transport(run.detail?.nilIfBlank ?? "The scheduled job failed. Try again or check its logs.")
    }
}
