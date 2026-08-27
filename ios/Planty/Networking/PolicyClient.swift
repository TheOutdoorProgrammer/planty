import Foundation

extension PlantyClient {
    func policies() async throws -> [OPAPolicy] {
        let response: PolicyListResponse = try await get(APIPath.listPolicies)
        return response.policies
    }

    func createPolicy(_ draft: PolicyDraft) async throws -> OPAPolicy {
        try await send("POST", APIPath.createPolicy, body: draft)
    }

    func updatePolicy(id: UUID, draft: PolicyDraft) async throws -> OPAPolicy {
        try await send("PUT", APIPath.updatePolicy(id: id.uuidString), body: draft)
    }

    func deletePolicy(id: UUID) async throws {
        _ = try await perform(try makeRequest("DELETE", APIPath.deletePolicy(id: id.uuidString)))
    }

    func previewPolicy(_ draft: PolicyDraft, plantSlug: String) async throws -> PolicyPreview {
        try await send(
            "POST",
            APIPath.previewPolicy,
            body: PolicyPreviewRequest(
                name: draft.name,
                description: draft.description,
                source: draft.source,
                mode: draft.mode,
                enabled: draft.enabled,
                plantSlug: plantSlug
            )
        )
    }

    func evaluatePolicy(id: UUID, plantSlug: String) async throws -> PolicyEvaluation {
        try await send(
            "POST",
            APIPath.evaluatePolicy(id: id.uuidString),
            body: PolicyEvaluationRequest(plantSlug: plantSlug)
        )
    }

    func policyEvaluations() async throws -> [PolicyEvaluation] {
        let response: PolicyEvaluationListResponse = try await get(APIPath.listPolicyEvaluations)
        return response.evaluations
    }

    func policyReference() async throws -> PolicyReference {
        try await get(APIPath.getPolicyReference)
    }
}
