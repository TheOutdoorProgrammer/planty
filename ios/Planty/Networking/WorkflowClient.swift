import Foundation

extension PlantyClient {
    func rechecks(slug: String) async throws -> [EvidenceWindow] {
        let response: EvidenceWindowListResponse = try await get(
            APIPath.listRechecks(slug: escaped(slug))
        )
        return response.rechecks ?? []
    }

    func proposeRecheck(slug: String, proposal: RecheckProposal) async throws -> EvidenceWindow {
        try await send("POST", APIPath.proposeRecheck(slug: escaped(slug)), body: proposal)
    }

    func evidenceWindow(id: UUID) async throws -> EvidenceWindow {
        try await get(APIPath.getEvidenceWindow(id: id.uuidString))
    }

    func startEvidenceWindow(id: UUID, request: EvidenceWindowStart) async throws -> EvidenceWindow {
        try await send("POST", APIPath.startEvidenceWindow(id: id.uuidString), body: request)
    }

    func reviewEvidenceWindow(id: UUID, request: EvidenceWindowReview) async throws -> EvidenceWindow {
        try await send("POST", APIPath.reviewEvidenceWindow(id: id.uuidString), body: request)
    }

    func concludeEvidenceWindow(id: UUID, request: EvidenceWindowConclusion) async throws -> EvidenceWindow {
        try await send("POST", APIPath.concludeEvidenceWindow(id: id.uuidString), body: request)
    }

    func cancelEvidenceWindow(id: UUID, request: EvidenceWindowCancellation) async throws -> EvidenceWindow {
        try await send("POST", APIPath.cancelEvidenceWindow(id: id.uuidString), body: request)
    }

    func guardrails(slug: String) async throws -> [EvidenceWindow] {
        let response: EvidenceWindowListResponse = try await get(APIPath.listGuardrails(slug: escaped(slug)))
        return response.guardrails ?? []
    }

    func overrideGuardrail(id: UUID, request: GuardrailOverrideRequest) async throws -> GuardrailOverride {
        try await send("POST", APIPath.overrideGuardrail(id: id.uuidString), body: request)
    }

    func experiments() async throws -> [EvidenceWindow] {
        let response: EvidenceWindowListResponse = try await get(APIPath.listExperiments)
        return response.experiments ?? []
    }

    func experiment(id: UUID) async throws -> EvidenceWindow {
        try await get(APIPath.getExperiment(id: id.uuidString))
    }

    func proposeExperiment(_ proposal: ExperimentProposal) async throws -> EvidenceWindow {
        try await send("POST", APIPath.proposeExperiment, body: proposal)
    }

    func incidentList(status: IncidentStatus? = nil) async throws -> [GardenIncident] {
        let query = status.map { [URLQueryItem(name: "status", value: $0.rawValue)] } ?? []
        let response: IncidentListResponse = try await get(APIPath.listIncidents, query: query)
        return response.incidents
    }

    func incident(id: UUID) async throws -> GardenIncident {
        try await get(APIPath.getIncident(id: id.uuidString))
    }

    func acknowledgeIncident(id: UUID, actor: String) async throws -> GardenIncident {
        try await send(
            "POST",
            APIPath.acknowledgeIncident(id: id.uuidString),
            body: IncidentActorRequest(actor: actor)
        )
    }

    func resolveIncident(id: UUID, request: IncidentResolutionRequest) async throws -> GardenIncident {
        try await send("POST", APIPath.resolveIncident(id: id.uuidString), body: request)
    }

    func evidenceCoverage() async throws -> [EvidenceCoverage] {
        let response: EvidenceCoverageResponse = try await get(APIPath.getEvidenceCoverage)
        return response.plants
    }
}
