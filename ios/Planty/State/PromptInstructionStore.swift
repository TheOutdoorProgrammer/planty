import Foundation
import Observation

@Observable
@MainActor
final class PromptInstructionStore {
    private(set) var instructions: [PromptInstructionSetting] = []
    private(set) var error: PlantyError?
    private(set) var hasLoaded = false
    private(set) var saving: Set<AIJob> = []

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
        instructions = []
        error = nil
        hasLoaded = false
        saving = []
    }

    func instruction(for job: AIJob) -> PromptInstructionSetting {
        instructions.first { $0.job == job }
            ?? PromptInstructionSetting(job: job, instructions: "", updatedAt: nil)
    }

    func loadIfNeeded() async {
        guard !hasLoaded else { return }
        await load()
    }

    func load() async {
        guard isConfigured else {
            instructions = []
            error = nil
            hasLoaded = true
            return
        }
        let startedGeneration = generation
        do {
            let loaded = try await api.promptInstructions()
            guard startedGeneration == generation else { return }
            instructions = loaded.filter { $0.job != .unknown }
            error = nil
            hasLoaded = true
        } catch {
            guard startedGeneration == generation, !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
            hasLoaded = true
        }
    }

    func save(_ draft: PromptOverlayDraft, for job: AIJob) async -> PlantyError? {
        guard draft.isValid else {
            return .transport("The overlay must contain text and stay under 12,000 bytes.")
        }
        return await write(job: job) {
            try await self.api.setPromptInstruction(job: job, instructions: draft.cleaned)
        }
    }

    func reset(_ job: AIJob) async -> PlantyError? {
        await write(job: job) { try await self.api.clearPromptInstruction(job: job) }
    }

    private func write(
        job: AIJob,
        operation: @escaping () async throws -> PromptInstructionSetting
    ) async -> PlantyError? {
        guard !saving.contains(job) else { return nil }
        saving.insert(job)
        defer { saving.remove(job) }
        do {
            let saved = try await operation()
            instructions.removeAll { $0.job == job }
            instructions.append(saved)
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
