import Foundation
import Testing

@testable import Planty

@Suite("Prompt overlays and explicit actuators")
struct SettingsAutomationTests {
    private let actuatorID = UUID(uuidString: "11111111-1111-1111-1111-111111111111")!
    private let leaseID = UUID(uuidString: "22222222-2222-2222-2222-222222222222")!

    @Test("Policy preview sends unsaved Rego and a real plant without enabling it")
    func policyPreviewIsSideEffectFree() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(json: """
            {"result":{"rules":[{"name":"needs_water","active":true,"value":{"reason":"Dry soil"}}],
             "notifications":[],"fan_runs":[],"agent":{"facts":[],"guidance":[],"deny_actions":[]}},
             "duration_ms":1.25}
            """)
        let draft = PolicyDraft(
            name: "Dry soil",
            description: "Warns from calibrated evidence.",
            source: PolicyDraft.fallbackExample,
            mode: .advisory,
            enabled: false
        )

        let preview = try await stub.client().previewPolicy(draft, plantSlug: "fern")
        let request = try #require(stub.requests.first)
        let json = try requestJSON(request)

        #expect(preview.result.rules.first?.name == "needs_water")
        #expect(preview.result.rules.first?.active == true)
        #expect(request.url?.path == "/v1/policies/preview")
        #expect(json["plant_slug"] as? String == "fern")
        #expect(json["source"] as? String == PolicyDraft.fallbackExample)
        #expect(json["enabled"] as? Bool == false)
    }

    @Test("Policy reference documents input, output, and safety from the service")
    func policyReferenceDecodes() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(json: """
            {"input_version":"planty.policy.input/v1","entrypoint":"data.planty.v1",
             "sections":[{"title":"Plant","fields":[{"path":"input.plant.age_days","type":"number",
             "description":"Whole days since acquisition."}]}],
             "output":[{"path":"needs_water | needs_misted","type":"any JSON value",
             "description":"Independent rules."}],
             "example":"package planty.v1","safety":["Preview never acts."]}
            """)

        let reference = try await stub.client().policyReference()

        #expect(reference.inputVersion == "planty.policy.input/v1")
        #expect(reference.entrypoint == "data.planty.v1")
        #expect(reference.sections.first?.fields.first?.path == "input.plant.age_days")
        #expect(reference.safety == ["Preview never acts."])
    }

    @Test("Prompt settings decode every job, including jobs without an overlay")
    func promptListDecodesAllJobs() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(json: """
            {"instructions":[
              {"job":"assess","instructions":"Prefer the newest photo.","updated_at":"2026-08-25T12:00:00Z"},
              {"job":"identify","instructions":""},
              {"job":"consult","instructions":""},
              {"job":"ask","instructions":""},
              {"job":"postmortem","instructions":""},
              {"job":"owner_update","instructions":""}
            ]}
            """)

        let instructions = try await stub.client().promptInstructions()

        #expect(instructions.count == 6)
        #expect(instructions.first?.hasOverlay == true)
        #expect(instructions.last?.job == .ownerUpdate)
        #expect(stub.requests.first?.url?.path == "/v1/prompt-instructions")
    }

    @Test("Saving sends only the editable overlay")
    func promptSaveBody() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(json: """
            {"job":"assess","instructions":"Keep this additional context.","updated_at":"2026-08-25T12:00:00Z"}
            """)

        _ = try await stub.client().setPromptInstruction(
            job: .assess,
            instructions: "Keep this additional context."
        )
        let request = try #require(stub.requests.first)
        let body = try #require(request.stubbedBody)
        let json = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])

        #expect(request.httpMethod == "PUT")
        #expect(request.url?.path == "/v1/prompt-instructions/assess")
        #expect(json.keys.sorted() == ["instructions"])
    }

    @Test("Overlay validation is nonblank and byte bounded")
    func promptDraftValidation() {
        #expect(!PromptOverlayDraft(instructions: "   ").isValid)
        #expect(PromptOverlayDraft(instructions: "Additional evidence guidance.").isValid)
        #expect(!PromptOverlayDraft(instructions: String(repeating: "🪴", count: 3_001)).isValid)
    }

    @Test("Registration sends the exact human-selected entity")
    func registrationIsExplicit() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(status: 201, json: actuatorJSON)

        _ = try await stub.client().registerActuator(
            ActuatorRegistration(
                entityID: "switch.grow_tent",
                name: "Grow tent exhaust",
                plantIDs: [actuatorID]
            )
        )
        let request = try #require(stub.requests.first)
        let body = try #require(request.stubbedBody)
        let json = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])

        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/actuators")
        #expect(json["entity_id"] as? String == "switch.grow_tent")
        #expect(json["name"] as? String == "Grow tent exhaust")
        #expect((json["plant_ids"] as? [String]) == [actuatorID.uuidString])
    }

    @Test("The store refuses an entity that discovery did not return")
    @MainActor
    func registrationCannotGuessAPlug() async {
        let stub = IsolatedStubTransport()
        let store = ActuatorStore(api: stub.client(), isConfigured: true)
        let guessed = HomeAssistantEntity(
            entityID: "switch.maybe_the_fan",
            friendlyName: "Maybe the fan",
            domain: "switch",
            deviceClass: nil,
            available: true,
            area: nil
        )

        let failure = await store.register(entity: guessed, name: "Tent fan", plantIDs: [actuatorID])

        #expect(failure != nil)
        #expect(stub.requests.isEmpty)
    }

    @Test("Start retries preserve the idempotency key and enforce one hour")
    @MainActor
    func boundedStartIsIdempotent() async throws {
        let stub = IsolatedStubTransport()
        let store = ActuatorStore(api: stub.client(), isConfigured: true)
        let actuator = try PlantyCoders.decoder().decode(Actuator.self, from: Data(actuatorJSON.utf8))

        let invalid = await store.start(actuator, durationSeconds: 3_601)
        #expect(invalid != nil)
        #expect(stub.requests.isEmpty)

        stub.respond(status: 500, json: #"{"error":"Home Assistant did not answer"}"#)
        _ = await store.start(actuator, durationSeconds: 600)
        let first = try requestJSON(try #require(stub.requests.first))
        stub.respond(routes: [
            "/v1/actuators/\(actuatorID.uuidString)/start": leaseJSON,
            "/v1/actuators/\(actuatorID.uuidString)/events": #"{"events":[],"count":0}"#
        ], status: 201)
        _ = await store.start(actuator, durationSeconds: 600)

        #expect(stub.requests.count == 2) // successful start also refreshes audit history
        let retry = try requestJSON(stub.requests[0])
        #expect(first["duration_seconds"] as? Int == 600)
        #expect(first["idempotency_key"] as? String == retry["idempotency_key"] as? String)
    }

    @Test("Stop addresses the Planty actuator ID and carries idempotency")
    func stopUsesPlantyID() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(json: #"{"stopped":false}"#)
        let key = UUID(uuidString: "33333333-3333-3333-3333-333333333333")!

        let response = try await stub.client().stopActuator(
            id: actuatorID,
            request: ActuatorStopRequest(actor: "owner", idempotencyKey: key)
        )
        let request = try #require(stub.requests.first)
        let json = try requestJSON(request)

        #expect(response.stopped == false)
        #expect(request.url?.path == "/v1/actuators/\(actuatorID.uuidString)/stop")
        #expect(json["idempotency_key"] as? String == key.uuidString)
    }

    @Test("Audit events decode provenance and newest-first timestamps")
    func auditEventDecodes() throws {
        let event = try PlantyCoders.decoder().decode(ActuatorEvent.self, from: Data("""
            {"id":"44444444-4444-4444-4444-444444444444","actuator_id":"\(actuatorID.uuidString)",
             "lease_id":"\(leaseID.uuidString)","action":"started","actor":"owner","source":"app",
             "detail":"Home Assistant accepted turn_on","created_at":"2026-08-25T12:00:01Z"}
            """.utf8))

        #expect(event.action == .started)
        #expect(event.actor == "owner")
        #expect(event.source == .app)
        #expect(event.leaseID == leaseID)
    }

    @Test("A shared fan is available from every assigned plant")
    func sharedFanAssignments() {
        let firstPlant = UUID()
        let secondPlant = UUID()
        let unrelatedPlant = UUID()
        let fan = Actuator(
            id: actuatorID,
            entityID: "fan.grow_room",
            name: "Grow room fan",
            kind: .fan,
            plantIDs: [firstPlant, secondPlant],
            createdAt: .now,
            updatedAt: .now,
            activeLease: nil
        )

        #expect([fan].assigned(to: firstPlant) == [fan])
        #expect([fan].assigned(to: secondPlant) == [fan])
        #expect([fan].assigned(to: unrelatedPlant).isEmpty)
        #expect(ActuatorRunDuration.allCases.map(\.rawValue) == [300, 600, 900, 1_800, 3_600])
        #expect(ObservationKind.airflow.label == "Airflow")
    }

    @Test("Light status stays unknown unless Home Assistant reported it")
    func lightStatusIsHonest() throws {
        let lightOn = try PlantyCoders.decoder().decode(Actuator.self, from: Data("""
            {"id":"\(actuatorID.uuidString)","entity_id":"light.grow","name":"Grow light",
             "kind":"light","plant_ids":["\(actuatorID.uuidString)"],"current_state":"on",
             "created_at":"2026-08-25T11:00:00Z","updated_at":"2026-08-25T11:00:00Z"}
            """.utf8))
        let unknown = try PlantyCoders.decoder().decode(Actuator.self, from: Data(actuatorJSON.utf8))

        #expect(lightOn.isOn == true)
        #expect(lightOn.stateLabel == "On")
        #expect(unknown.isOn == nil)
        #expect(unknown.stateLabel == "Status unknown")
    }

    @Test("Light schedules preserve their timezone and reject a zero-length window")
    func lightScheduleDraftIsValid() {
        var draft = LightScheduleDraft(schedule: nil, defaultTimezone: "America/New_York")
        #expect(draft.canSave)
        #expect(draft.timezone == "America/New_York")
        draft.endMinute = draft.startMinute
        #expect(!draft.canSave)

        let saved = LightSchedule(
            actuatorID: actuatorID,
            startMinute: 480,
            endMinute: 1_200,
            timezone: "America/Chicago",
            enabled: false,
            lastAppliedState: nil,
            lastAppliedAt: nil,
            lastError: nil,
            createdAt: .now,
            updatedAt: .now
        )
        let loaded = LightScheduleDraft(schedule: saved, defaultTimezone: "America/New_York")
        #expect(loaded.startMinute == 480)
        #expect(loaded.endMinute == 1_200)
        #expect(loaded.timezone == "America/Chicago")
        #expect(!loaded.enabled)
    }

    @Test("A successful light command updates the dashboard immediately")
    @MainActor
    func lightControlUpdatesState() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(json: """
            {"actuators":[{"id":"\(actuatorID.uuidString)","entity_id":"light.grow",
             "name":"Grow light","kind":"light","plant_ids":["\(actuatorID.uuidString)"],
             "current_state":"off","created_at":"2026-08-25T11:00:00Z",
             "updated_at":"2026-08-25T11:00:00Z"}],"count":1}
            """)
        let store = ActuatorStore(api: stub.client(), isConfigured: true)
        await store.load(includeEvents: false)
        let light = try #require(store.registered.first)
        stub.respond(json: #"{"on":true}"#)

        let failure = await store.setLight(light, isOn: true)

        #expect(failure == nil)
        #expect(store.registered.first?.isOn == true)
        #expect(stub.requests.last?.url?.path == "/v1/actuators/\(actuatorID.uuidString)/state")
    }

    private var actuatorJSON: String {
        """
        {"id":"\(actuatorID.uuidString)","entity_id":"switch.grow_tent","name":"Grow tent exhaust",
         "kind":"switch","plant_ids":["\(actuatorID.uuidString)"],
         "created_at":"2026-08-25T11:00:00Z","updated_at":"2026-08-25T11:00:00Z"}
        """
    }

    private var leaseJSON: String {
        """
        {"id":"\(leaseID.uuidString)","actuator_id":"\(actuatorID.uuidString)","requested_seconds":600,
         "deadline":"2026-08-25T12:10:00Z","actor":"owner","source":"app",
         "idempotency_key":"55555555-5555-5555-5555-555555555555","started_at":"2026-08-25T12:00:00Z",
         "created_at":"2026-08-25T12:00:00Z"}
        """
    }

    private func requestJSON(_ request: URLRequest) throws -> [String: Any] {
        let body = try #require(request.stubbedBody)
        return try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
    }
}
