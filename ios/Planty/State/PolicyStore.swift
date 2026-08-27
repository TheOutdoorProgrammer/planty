import Foundation
import Observation

@Observable
@MainActor
final class PolicyStore {
    private(set) var policies: [OPAPolicy] = []
    private(set) var evaluations: [PolicyEvaluation] = []
    private(set) var reference: PolicyReference?
    private(set) var error: PlantyError?
    private(set) var hasLoaded = false
    private(set) var isWriting = false

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
        policies = []
        evaluations = []
        reference = nil
        error = nil
        hasLoaded = false
        isWriting = false
    }

    func load() async {
        guard isConfigured else { hasLoaded = true; return }
        let startedGeneration = generation
        do {
            async let loadedPolicies = api.policies()
            async let loadedEvaluations = api.policyEvaluations()
            async let loadedReference = api.policyReference()
            let result = try await (loadedPolicies, loadedEvaluations, loadedReference)
            guard startedGeneration == generation else { return }
            policies = result.0.sorted { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
            evaluations = result.1
            reference = result.2
            error = nil
            hasLoaded = true
        } catch {
            guard startedGeneration == generation, !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
            hasLoaded = true
        }
    }

    func save(id: UUID?, draft: PolicyDraft) async -> OPAPolicy? {
        guard draft.isValid, !isWriting else { return nil }
        isWriting = true
        defer { isWriting = false }
        do {
            let saved: OPAPolicy
            if let id {
                saved = try await api.updatePolicy(id: id, draft: draft)
            } else {
                saved = try await api.createPolicy(draft)
            }
            policies.removeAll { $0.id == saved.id }
            policies.append(saved)
            policies.sort { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
            error = nil
            return saved
        } catch {
            self.error = PlantyError.from(error)
            return nil
        }
    }

    func delete(_ policy: OPAPolicy) async -> Bool {
        do {
            try await api.deletePolicy(id: policy.id)
            policies.removeAll { $0.id == policy.id }
            return true
        } catch {
            self.error = PlantyError.from(error)
            return false
        }
    }

    func preview(_ draft: PolicyDraft, plantSlug: String) async -> PolicyPreview? {
        do {
            let preview = try await api.previewPolicy(draft, plantSlug: plantSlug)
            error = nil
            return preview
        } catch {
            self.error = PlantyError.from(error)
            return nil
        }
    }

    func evaluate(_ policy: OPAPolicy, plantSlug: String) async -> PolicyEvaluation? {
        do {
            let evaluation = try await api.evaluatePolicy(id: policy.id, plantSlug: plantSlug)
            evaluations.removeAll { $0.id == evaluation.id }
            evaluations.insert(evaluation, at: 0)
            error = nil
            return evaluation
        } catch {
            self.error = PlantyError.from(error)
            return nil
        }
    }
}
