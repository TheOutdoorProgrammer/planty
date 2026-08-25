import Foundation
import Testing

@testable import Planty

@Suite("Evidence workflows and incident radar")
struct WorkflowClientTests {
    private let plantID = UUID(uuidString: "0651DE3F-6EC5-4A7B-9981-8CF8F53D0F4D")!
    private let windowID = UUID(uuidString: "11111111-1111-1111-1111-111111111111")!
    private let photoID = UUID(uuidString: "22222222-2222-2222-2222-222222222222")!

    @Test("Recheck proposals reference an existing baseline photo")
    func proposeRecheckUsesLedgerReference() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(status: 201, json: windowJSON)
        let proposal = RecheckProposal(
            interventionKind: .watered,
            baseline: [.init(plantID: plantID, kind: .photo, id: photoID)],
            expected: [.init(plantID: plantID, kind: .photo, instruction: "Same angle")],
            earliestReviewAt: try #require(PlantyDateFormat.date(from: "2026-08-25T14:00:00Z")),
            latestReviewAt: try #require(PlantyDateFormat.date(from: "2026-08-28T12:00:00Z")),
            actor: "owner"
        )

        let saved = try await stub.client().proposeRecheck(slug: "mona", proposal: proposal)
        let request = try #require(stub.requests.first)
        let body = try json(request)
        let baseline = try #require(body["baseline"] as? [[String: Any]])

        #expect(saved.guardrail?.conflictingKinds == [.watered])
        #expect(request.url?.path == "/v1/plants/mona/rechecks")
        #expect(baseline.first?["id"] as? String == photoID.uuidString)
        #expect(baseline.first?["phase"] == nil)
    }

    @Test("Rechecks restore from their plant route after relaunch")
    func listRechecksRestoresPersistedWindows() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(json: """
            {"rechecks":[\(windowJSON)],"count":1}
            """)

        let restored = try await stub.client().rechecks(slug: "mona")

        #expect(restored.map(\.id) == [windowID])
        #expect(stub.requests.first?.url?.path == "/v1/plants/mona/rechecks")
    }

    @Test("Review evidence posts to the window and cannot invent a phase")
    func reviewUsesGeneratedWindowRoute() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(json: windowJSON.replacingOccurrences(of: "\"status\":\"proposed\"", with: "\"status\":\"ready\""))
        let reviewID = UUID(uuidString: "33333333-3333-3333-3333-333333333333")!

        _ = try await stub.client().reviewEvidenceWindow(
            id: windowID,
            request: EvidenceWindowReview(evidence: [.init(plantID: plantID, kind: .photo, id: reviewID)])
        )
        let request = try #require(stub.requests.first)
        let body = try json(request)
        let evidence = try #require(body["evidence"] as? [[String: Any]])

        #expect(request.url?.path == "/v1/evidence-windows/\(windowID.uuidString)/review")
        #expect(evidence.first?["phase"] == nil)
    }

    @Test("Incident decoding preserves each plant action and sparse evidence")
    func incidentPreservesActions() throws {
        let incident = try PlantyCoders.decoder().decode(GardenIncident.self, from: Data(incidentJSON.utf8))

        #expect(incident.plants.count == 1)
        #expect(incident.plants.first?.action == .urgent)
        #expect(incident.evidence.sensorLinkIDs.isEmpty)
        #expect(incident.evidence.note == "Two signals moved together.")
    }

    @Test("Resolution uses the contract outcome and Planty incident ID")
    func incidentResolutionRoute() async throws {
        let stub = IsolatedStubTransport()
        stub.respond(json: incidentJSON.replacingOccurrences(of: "\"status\":\"open\"", with: "\"status\":\"resolved\""))
        let incidentID = UUID(uuidString: "44444444-4444-4444-4444-444444444444")!

        _ = try await stub.client().resolveIncident(
            id: incidentID,
            request: IncidentResolutionRequest(
                outcome: .inconclusive,
                actor: "owner",
                conclusion: "Timing matched, but the evidence did not establish cause."
            )
        )
        let request = try #require(stub.requests.first)
        let body = try json(request)

        #expect(request.url?.path == "/v1/incidents/\(incidentID.uuidString)/resolve")
        #expect(body["outcome"] as? String == "inconclusive")
        #expect(body["actor"] as? String == "owner")
    }

    @Test("Coverage exposes exactly the backend-ranked next input")
    @MainActor
    func coverageKeepsOnePriority() async {
        let stub = IsolatedStubTransport()
        stub.respond(json: """
            {"plants":[{"plant":\(ModelDecodingTests.monaJSON),"photo_count":0,"sensor_count":0,
             "has_soil_sensor":false,"soil_calibrated":false,"botanical_known":true,
             "toxicity_checked":true,"health_established":false,
             "next_best_input":"Take a whole-plant baseline photo",
             "why":"A later visual change needs something honest to compare against."}],
             "count":1,"complete":0}
            """)
        let store = EvidenceWorkflowStore(api: stub.client(), isConfigured: true)

        await store.loadCoverage()

        #expect(store.coverage.count == 1)
        #expect(store.nextBestCoverage?.nextBestInput == "Take a whole-plant baseline photo")
        #expect(stub.requests.first?.url?.path == "/v1/evidence-coverage")
    }

    private var windowJSON: String {
        """
        {"id":"\(windowID.uuidString)","kind":"recheck","status":"proposed",
         "plant_ids":["\(plantID.uuidString)"],"intervention_kind":"watered",
         "baseline":[{"plant_id":"\(plantID.uuidString)","kind":"photo","id":"\(photoID.uuidString)","phase":"baseline"}],
         "expected":[{"plant_id":"\(plantID.uuidString)","kind":"photo","instruction":"Same angle"}],"review":[],
         "earliest_review_at":"2026-08-25T14:00:00Z","latest_review_at":"2026-08-28T12:00:00Z",
         "proposed_by":"app","proposed_actor":"owner",
         "guardrail":{"reason":"Wait for delivery evidence before watering again.","conflicting_kinds":["watered"],"red_flags":["rapid collapse"]},
         "overrides":[],"created_at":"2026-08-25T12:00:00Z","updated_at":"2026-08-25T12:00:00Z"}
        """
    }

    private var incidentJSON: String {
        """
        {"id":"44444444-4444-4444-4444-444444444444","status":"open",
         "suspected_factor_type":"location","suspected_factor_ref":"Living room",
         "summary":"Two plants declined together.","confidence":0.7,
         "evidence":{"run_id":"55555555-5555-5555-5555-555555555555","verdict_ids":["66666666-6666-6666-6666-666666666666"],"note":"Two signals moved together."},
         "detected_run_id":"55555555-5555-5555-5555-555555555555",
         "plants":[{"plant":\(ModelDecodingTests.monaJSON),"role":"affected","verdict_id":"66666666-6666-6666-6666-666666666666","action":"urgent","confidence":0.8,"first_seen_at":"2026-08-25T10:00:00Z","last_seen_at":"2026-08-25T12:00:00Z"}],
         "first_seen_at":"2026-08-25T10:00:00Z","last_seen_at":"2026-08-25T12:00:00Z",
         "created_at":"2026-08-25T12:00:00Z","updated_at":"2026-08-25T12:00:00Z"}
        """
    }

    private func json(_ request: URLRequest) throws -> [String: Any] {
        let data = try #require(request.stubbedBody)
        return try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
    }
}
