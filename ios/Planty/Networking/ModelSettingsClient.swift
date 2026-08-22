import Foundation

// These read and write configuration rather than waiting on a model, so they
// use the ordinary timeout rather than the patient one.
extension PlantyClient {
    func aiModels() async throws -> [AIModel] {
        let list: AIModelList = try await get(APIPath.listModels)
        return list.models
    }

    func jobAssignments() async throws -> [JobAssignment] {
        let list: JobAssignmentList = try await get(APIPath.listModelAssignments)
        return list.assignments
    }

    func assign(job: AIJob, provider: String, model: String) async throws -> JobAssignment {
        try await send(
            "PUT",
            APIPath.setModelAssignment(job: job.rawValue),
            body: ModelAssignmentRequest(provider: provider, model: model)
        )
    }

    func clearAssignment(job: AIJob) async throws -> JobAssignment {
        try await send("DELETE", APIPath.clearModelAssignment(job: job.rawValue), body: Empty())
    }
}

private struct Empty: Encodable {}
