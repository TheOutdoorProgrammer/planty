import Foundation
import Testing

@testable import Planty

@Suite("Choosing which model answers what", .serialized)
struct ModelSettingsTests {
    private static let catalog = """
        {
          "models": [
            {"provider":"claude","id":"claude-opus-5","ref":"claude/claude-opus-5",
             "name":"Claude Opus 5","rank":1,
             "skills":{"vision":true,"schema":true,"tools":true,"offered_photos":true},
             "jobs":["assess","identify","consult","ask","postmortem","owner_update"]},
            {"provider":"opencode-go","id":"qwen3.8-max","ref":"opencode-go/qwen3.8-max",
             "name":"Qwen3.8 Max","rank":11,"note":"Best at photographs.",
             "skills":{"vision":true,"schema":true,"tools":true,"offered_photos":true},
             "jobs":["assess","identify","consult","ask","postmortem","owner_update"]},
            {"provider":"opencode-go","id":"deepseek-v4-flash","ref":"opencode-go/deepseek-v4-flash",
             "name":"DeepSeek V4 Flash","rank":19,"note":"Text only.",
             "skills":{"vision":false,"schema":true,"tools":true,"offered_photos":true},
             "jobs":["assess","postmortem","owner_update"]}
          ]
        }
        """

    @Test("The client reads the catalogue from one route")
    func clientLoadsCatalogue() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(json: Self.catalog)

        let models = try await stub.client().aiModels()
        let request = try #require(stub.requests.first)

        #expect(request.httpMethod == "GET")
        #expect(request.url?.path == "/v1/models")
        #expect(models.map(\.ref) == [
            "claude/claude-opus-5", "opencode-go/qwen3.8-max", "opencode-go/deepseek-v4-flash"
        ])
        #expect(models.first?.name == "Claude Opus 5")
        #expect(models.last?.skills.vision == false)
    }

    @Test("Assignments decode, including which jobs are still on their default")
    func assignmentsDecode() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(json: """
            {"assignments":[
              {"job":"assess","provider":"opencode-go","model":"deepseek-v4-flash",
               "ref":"opencode-go/deepseek-v4-flash","default":false},
              {"job":"identify","default":true},
              {"job":"consult","default":true,"offered_photos":false}
            ]}
            """)

        let assignments = try await stub.client().jobAssignments()
        #expect(assignments.first?.job == .assess)
        #expect(assignments.first?.isDefault == false)
        #expect(assignments[1].job == .identify)
        #expect(assignments[1].isDefault == true)
        #expect(assignments[1].ref == nil)
        #expect(assignments.last?.offeredPhotos == false)
    }

    @Test("The store exposes when the selected consultation provider cannot open history")
    func historicalPhotoCapability() async {
        let stub = IsolatedStubTransport()
        stub.respond(routes: [
            "/v1/models": Self.catalog,
            "/v1/model-assignments": #"{"assignments":[{"job":"consult","default":true,"offered_photos":false}]}"#
        ])
        let store = await ModelSettingsStore(api: stub.client(), isConfigured: true)
        await store.load()

        #expect(await store.canInspectHistoricalPhotos(for: .consult) == false)
    }

    @Test("Assigning a job sends provider and model to that job's route")
    func assigningSendsTheChoice() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(json: """
            {"job":"identify","provider":"opencode-go","model":"qwen3.8-max",
             "ref":"opencode-go/qwen3.8-max","default":false}
            """)

        let saved = try await stub.client().assign(
            job: .identify, provider: "opencode-go", model: "qwen3.8-max"
        )
        let request = try #require(stub.requests.first)

        #expect(request.httpMethod == "PUT")
        #expect(request.url?.path == "/v1/model-assignments/identify")
        #expect(saved.ref == "opencode-go/qwen3.8-max")

        let body = try #require(request.stubbedBody)
        let sent = try #require(try JSONSerialization.jsonObject(with: body) as? [String: Any])
        #expect(sent["provider"] as? String == "opencode-go")
        #expect(sent["model"] as? String == "qwen3.8-max")
    }

    // The gating rule, seen from the phone: a job that shows a photograph must
    // not even offer a model that cannot read one.
    @Test("A job only offers models the server says can do it")
    func incapableModelsAreAbsent() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(routes: [
            "/v1/models": Self.catalog,
            "/v1/model-assignments": #"{"assignments":[{"job":"identify","default":true}]}"#
        ])

        let store = await ModelSettingsStore(api: stub.client(), isConfigured: true)
        await store.load()

        let forIdentify = await store.choices(for: .identify).map(\.ref)
        #expect(!forIdentify.contains("opencode-go/deepseek-v4-flash"))
        #expect(forIdentify.contains("opencode-go/qwen3.8-max"))

        let forAssess = await store.choices(for: .assess).map(\.ref)
        #expect(forAssess.contains("opencode-go/deepseek-v4-flash"))
    }

    @Test("An unconfigured app offers nothing rather than failing")
    func unconfiguredIsEmpty() async throws {
        let stub = IsolatedStubTransport()
        let store = await ModelSettingsStore(api: stub.client(), isConfigured: false)
        await store.load()

        #expect(await store.models.isEmpty)
        #expect(await store.hasLoaded)
        #expect(await store.error == nil)
    }

    @Test("Every job carries a label and an explanation")
    func everyJobReads() {
        for job in AIJob.allCases where job != .unknown {
            #expect(!job.label.isEmpty)
            #expect(!job.explanation.isEmpty)
        }
    }
}
