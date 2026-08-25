import Foundation
import Testing

@testable import Planty

@Suite("Phone-triggered scheduled jobs")
struct ScheduledJobTests {
    @Test("The app lists code-owned jobs with their latest Kubernetes run")
    func listDecodes() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(json: """
            {"jobs":[{
              "id":"daily","name":"Check every plant",
              "purpose":"Create a fresh evidence-backed daily assessment.",
              "category":"Care","cadence":"Daily at 8:00 AM",
              "schedule":"0 8 * * *","time_zone":"America/New_York","suspended":false,
              "latest_run":{"id":"planty-manual-daily-test","job":"daily","state":"succeeded",
                "started_at":"2026-08-25T12:00:00Z","completed_at":"2026-08-25T12:01:00Z"}
            }]}
            """)

        let jobs = try await stub.client().scheduledJobs()

        #expect(jobs.count == 1)
        #expect(jobs[0].id == .daily)
        #expect(jobs[0].cadence == "Daily at 8:00 AM")
        #expect(jobs[0].latestRun?.state == .succeeded)
        #expect(stub.requests.first?.url?.path == "/v1/scheduled-jobs")
    }

    @Test("Run now addresses a fixed job ID and needs no command body")
    func startUsesAllowlistedPath() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(status: 202, json: """
            {"id":"planty-manual-daily-test","job":"daily","state":"queued"}
            """)

        let run = try await stub.client().runScheduledJob(.daily)
        let request = try #require(stub.requests.first)

        #expect(run.state == .queued)
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/scheduled-jobs/daily/runs")
    }

    @Test("A completed fresh check resolves without waiting for another schedule")
    @MainActor
    func completedRunReturnsSuccess() async {
        let stub = IsolatedStubTransport()
        stub.respond(status: 200, json: """
            {"id":"planty-manual-daily-test","job":"daily","state":"succeeded",
             "started_at":"2026-08-25T12:00:00Z","completed_at":"2026-08-25T12:01:00Z"}
            """)
        let store = ScheduledJobStore(api: stub.client(), isConfigured: true, pollInterval: .zero)

        let failure = await store.run(.daily)

        #expect(failure == nil)
        #expect(!store.isRunning(.daily))
    }

    @Test("A failed Kubernetes run is actionable in the app")
    @MainActor
    func failedRunReturnsItsReason() async {
        let stub = IsolatedStubTransport()
        stub.respond(status: 200, json: """
            {"id":"planty-manual-daily-test","job":"daily","state":"failed",
             "detail":"The model backend did not answer."}
            """)
        let store = ScheduledJobStore(api: stub.client(), isConfigured: true, pollInterval: .zero)

        let failure = await store.run(.daily)

        #expect(failure?.errorDescription?.contains("model backend") == true)
        #expect(store.error == failure)
    }
}
