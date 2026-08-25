import Foundation

extension PlantyClient {
    func scheduledJobs() async throws -> [ScheduledJob] {
        let response: ScheduledJobListResponse = try await get(APIPath.listScheduledJobs)
        return response.jobs
    }

    func runScheduledJob(_ job: ScheduledJobID) async throws -> ScheduledJobRun {
        try await send(
            "POST",
            APIPath.runScheduledJob(job: job.rawValue),
            body: ScheduledJobRunRequest()
        )
    }
}

private struct ScheduledJobRunRequest: Encodable {}
