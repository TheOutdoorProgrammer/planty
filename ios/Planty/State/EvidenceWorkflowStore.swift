import Foundation
import Observation

@Observable
@MainActor
final class EvidenceWorkflowStore {
    private(set) var windows: [UUID: EvidenceWindow] = [:]
    private(set) var experimentIDs: [UUID] = []
    private(set) var guardrailIDsByPlant: [UUID: [UUID]] = [:]
    private(set) var latestPhotos: [UUID: Photo] = [:]
    private(set) var ledgers: [UUID: PlantTimeline] = [:]
    private(set) var coverage: [EvidenceCoverage] = []
    private(set) var error: PlantyError?
    private(set) var isWorking = false

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
        windows = [:]
        experimentIDs = []
        guardrailIDsByPlant = [:]
        latestPhotos = [:]
        ledgers = [:]
        coverage = []
        error = nil
        isWorking = false
    }

    func windows(for plant: Plant) -> [EvidenceWindow] {
        windows.values.filter { $0.plantIDs.contains(plant.id) && $0.kind == .recheck }
            .sorted { $0.updatedAt > $1.updatedAt }
    }

    func guardrails(for plant: Plant) -> [EvidenceWindow] {
        (guardrailIDsByPlant[plant.id] ?? []).compactMap { windows[$0] }
    }

    var experiments: [EvidenceWindow] {
        experimentIDs.compactMap { windows[$0] }.sorted { $0.updatedAt > $1.updatedAt }
    }

    func window(id: UUID) -> EvidenceWindow? { windows[id] }

    func ledger(for plant: Plant) -> PlantTimeline {
        ledgers[plant.id] ?? PlantTimeline()
    }

    var nextBestCoverage: EvidenceCoverage? {
        coverage.first { $0.nextBestInput?.nilIfBlank != nil && $0.why?.nilIfBlank != nil }
    }

    func loadCoverage() async {
        guard isConfigured else { coverage = []; return }
        await perform { self.coverage = try await self.api.evidenceCoverage() }
    }

    func loadGuardrails(for plant: Plant) async {
        guard isConfigured else { return }
        await perform {
            let loaded = try await self.api.guardrails(slug: plant.slug)
            loaded.forEach(self.upsert)
            self.guardrailIDsByPlant[plant.id] = loaded.map(\.id)
        }
    }

    func loadRechecks(for plant: Plant) async {
        guard isConfigured else { return }
        await perform {
            let loaded = try await self.api.rechecks(slug: plant.slug)
            loaded.forEach(self.upsert)
        }
    }

    func loadExperiments() async {
        guard isConfigured else { return }
        await perform {
            let loaded = try await self.api.experiments()
            loaded.forEach(self.upsert)
            self.experimentIDs = loaded.map(\.id)
        }
    }

    func loadLatestPhotos(for plants: [Plant]) async {
        await loadLedgers(for: plants)
    }

    func loadLedgers(for plants: [Plant]) async {
        let startedGeneration = generation
        let client = api
        for plant in plants {
            do {
                async let timeline = client.timeline(slug: plant.slug)
                async let detail = client.plant(slug: plant.slug)
                let (timelineValue, detailValue) = try await (timeline, detail)
                let ledger = timelineValue.merging(detailValue)
                guard startedGeneration == generation else { return }
                ledgers[plant.id] = ledger
                if let photo = ledger.photos.max(by: { $0.takenAt < $1.takenAt }) {
                    latestPhotos[plant.id] = photo
                } else {
                    latestPhotos.removeValue(forKey: plant.id)
                }
            } catch {
                guard !PlantyError.isCancellation(error) else { return }
                self.error = PlantyError.from(error)
            }
        }
    }

    func proposeRecheck(for plant: Plant, proposal: RecheckProposal) async -> PlantyError? {
        await write { try await self.api.proposeRecheck(slug: plant.slug, proposal: proposal) }
    }

    func proposeExperiment(_ proposal: ExperimentProposal) async -> PlantyError? {
        let failure = await write { try await self.api.proposeExperiment(proposal) }
        if failure == nil, let newest = windows.values.max(by: { $0.createdAt < $1.createdAt }) {
            if !experimentIDs.contains(newest.id) { experimentIDs.insert(newest.id, at: 0) }
        }
        return failure
    }

    func start(_ window: EvidenceWindow, observationID: UUID) async -> PlantyError? {
        await write {
            try await self.api.startEvidenceWindow(
                id: window.id,
                request: EvidenceWindowStart(observationID: observationID, actor: "owner")
            )
        }
    }

    func review(_ window: EvidenceWindow, evidence: [EvidenceReferenceRequest]) async -> PlantyError? {
        await write {
            try await self.api.reviewEvidenceWindow(
                id: window.id,
                request: EvidenceWindowReview(evidence: evidence)
            )
        }
    }

    func conclude(_ window: EvidenceWindow, outcome: EvidenceWindowOutcome, conclusion: String) async -> PlantyError? {
        await write {
            try await self.api.concludeEvidenceWindow(
                id: window.id,
                request: EvidenceWindowConclusion(outcome: outcome, conclusion: conclusion.cleaned, actor: "owner")
            )
        }
    }

    func cancel(_ window: EvidenceWindow, reason: String) async -> PlantyError? {
        let reason = reason.cleaned
        guard !reason.isEmpty else { return .transport("Explain why the evidence window is being cancelled.") }
        return await write {
            try await self.api.cancelEvidenceWindow(
                id: window.id,
                request: EvidenceWindowCancellation(reason: reason, actor: "owner")
            )
        }
    }

    func override(_ window: EvidenceWindow, plant: Plant, kind: ObservationKind, reason: String) async -> PlantyError? {
        let reason = reason.cleaned
        guard !reason.isEmpty else { return .transport("Explain why the guardrail must be overridden.") }
        return await performWrite {
            _ = try await self.api.overrideGuardrail(
                id: window.id,
                request: GuardrailOverrideRequest(plantID: plant.id, kind: kind, reason: reason, actor: "owner")
            )
            return try await self.api.evidenceWindow(id: window.id)
        }
    }

    private func write(_ operation: @escaping () async throws -> EvidenceWindow) async -> PlantyError? {
        await performWrite(operation)
    }

    private func performWrite(_ operation: @escaping () async throws -> EvidenceWindow) async -> PlantyError? {
        guard !isWorking else { return nil }
        isWorking = true
        defer { isWorking = false }
        do {
            upsert(try await operation())
            error = nil
            return nil
        } catch {
            guard !PlantyError.isCancellation(error) else { return nil }
            let failure = PlantyError.from(error)
            self.error = failure
            return failure
        }
    }

    private func perform(_ operation: () async throws -> Void) async {
        do {
            try await operation()
            error = nil
        } catch {
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
    }

    private func upsert(_ window: EvidenceWindow) { windows[window.id] = window }
}
