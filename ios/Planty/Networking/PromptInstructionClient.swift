import Foundation

extension PlantyClient {
    func promptInstructions() async throws -> [PromptInstructionSetting] {
        let response: PromptInstructionListResponse = try await get(APIPath.listPromptInstructions)
        return response.instructions
    }

    func setPromptInstruction(job: AIJob, instructions: String) async throws -> PromptInstructionSetting {
        try await send(
            "PUT",
            APIPath.setPromptInstruction(job: escaped(job.rawValue)),
            body: PromptInstructionUpdate(instructions: instructions)
        )
    }

    func clearPromptInstruction(job: AIJob) async throws -> PromptInstructionSetting {
        let path = APIPath.clearPromptInstruction(job: escaped(job.rawValue))
        return try decode(PromptInstructionSetting.self, from: try await perform(try makeRequest("DELETE", path)))
    }
}
