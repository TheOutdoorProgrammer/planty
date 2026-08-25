import Foundation
import Testing

@testable import Planty

@Suite("Plant health evidence")
struct PlantHealthTests {
    private let plantID = UUID(uuidString: "11111111-1111-1111-1111-111111111111")!
    private let baselineID = UUID(uuidString: "22222222-2222-2222-2222-222222222222")!
    private let correctionID = UUID(uuidString: "33333333-3333-3333-3333-333333333333")!

    private var baselineJSON: String {
        """
        {
          "id":"\(baselineID.uuidString)",
          "plant_id":"\(plantID.uuidString)",
          "score":70,
          "rationale":"Leaves are full and upright.",
          "evidence":{"summary":"Compared the whole plant with last week."},
          "source":"app",
          "actor":"owner",
          "idempotency_key":"44444444-4444-4444-4444-444444444444",
          "created_at":"2026-08-24T12:00:00Z"
        }
        """
    }

    private var correctionJSON: String {
        """
        {
          "id":"\(correctionID.uuidString)",
          "plant_id":"\(plantID.uuidString)",
          "score":65,
          "requested_delta":-8,
          "applied_delta":-5,
          "rationale":"Two lower leaves yellowed.",
          "evidence":{
            "photo_ids":["55555555-5555-5555-5555-555555555555"],
            "summary":"New yellowing is visible.",
            "model_version":"claude-opus-5"
          },
          "source":"agent",
          "actor":"daily assessment",
          "judgment_run_id":"66666666-6666-6666-6666-666666666666",
          "created_at":"2026-08-25T12:00:00Z"
        }
        """
    }

    @Test("No current event decodes as unknown rather than zero")
    func unknownDecodes() async throws {
        let transport = IsolatedStubTransport()
        transport.respond(json: #"{"current":null,"events":[],"count":0}"#)

        let response = try await transport.client().plantHealth(slug: "mona")

        #expect(response.current == nil)
        #expect(response.events.isEmpty)
        #expect(HealthPresentation(score: response.current?.score).title == "Health unknown")
    }

    @Test("Health event decodes deltas and sparse evidence")
    func eventDecodes() async throws {
        let data = Data(correctionJSON.utf8)
        let event = try PlantyCoders.decoder().decode(HealthEvent.self, from: data)

        #expect(event.score == 65)
        #expect(event.requestedDelta == -8)
        #expect(event.appliedDelta == -5)
        #expect(event.evidence.photoIDs.count == 1)
        #expect(event.evidence.observationIDs.isEmpty)
        #expect(event.evidence.readingIDs.isEmpty)
        #expect(event.source == .agent)
    }

    @Test("The client sends an idempotent evidence-backed correction")
    func clientAddsCorrection() async throws {
        let transport = IsolatedStubTransport()
        transport.respond(status: 201, json: correctionJSON)
        let idempotencyKey = UUID(uuidString: "77777777-7777-7777-7777-777777777777")!
        let change = NewHealthChange(
            kind: .delta,
            value: -8,
            rationale: "Two lower leaves yellowed.",
            evidence: HealthEvidence(summary: "New yellowing is visible."),
            actor: "owner",
            idempotencyKey: idempotencyKey
        )

        _ = try await transport.client().addHealthEvent(slug: "mona fern", change: change)
        let request = try #require(transport.requests.first)
        let body = try #require(request.stubbedBody)
        let json = try #require(JSONSerialization.jsonObject(with: body) as? [String: Any])
        let evidence = try #require(json["evidence"] as? [String: Any])

        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/plants/mona%20fern/health-events")
        #expect(json["kind"] as? String == "delta")
        #expect(json["value"] as? Double == -8)
        #expect(json["idempotency_key"] as? String == idempotencyKey.uuidString)
        #expect(evidence["summary"] as? String == "New yellowing is visible.")
    }

    @Test("The store sorts newest first and adopts it as current")
    @MainActor
    func storeOrdersHistory() async {
        let transport = IsolatedStubTransport()
        transport.respond(json: """
            {"current":\(baselineJSON),"events":[\(baselineJSON),\(correctionJSON)],"count":2}
            """)
        let plant = Plant.fixture(slug: "mona", id: plantID)
        let store = PlantHealthStore(api: transport.client())

        await store.load(plant)

        #expect(store.hasLoaded(plant))
        #expect(store.current(for: plant)?.id == correctionID)
        #expect(store.history(for: plant).map(\.id) == [correctionID, baselineID])
    }

    @Test("Saving the same returned event cannot duplicate history")
    @MainActor
    func saveIsLocallyIdempotent() async throws {
        let transport = IsolatedStubTransport()
        transport.respond(status: 201, json: baselineJSON)
        let plant = Plant.fixture(slug: "mona", id: plantID)
        let store = PlantHealthStore(api: transport.client())
        let request = try #require(
            HealthAdjustmentDraft(
                kind: .baseline,
                value: "70",
                rationale: "Leaves are full and upright.",
                evidenceSummary: "Compared the whole plant with last week."
            ).request(idempotencyKey: UUID())
        )

        _ = await store.save(request, for: plant)
        _ = await store.save(request, for: plant)

        #expect(store.history(for: plant).map(\.id) == [baselineID])
        #expect(store.current(for: plant)?.score == 70)
    }

    @Test("Manual drafts require valid score, rationale, and evidence")
    func draftValidation() {
        let key = UUID()
        #expect(HealthAdjustmentDraft(kind: .baseline).request(idempotencyKey: key) == nil)
        #expect(HealthAdjustmentDraft(
            kind: .baseline,
            value: "101",
            rationale: "Observed it.",
            evidenceSummary: "Whole plant photo."
        ).request(idempotencyKey: key) == nil)
        #expect(HealthAdjustmentDraft(
            kind: .delta,
            value: "0",
            rationale: "Observed it.",
            evidenceSummary: "Whole plant photo."
        ).request(idempotencyKey: key) == nil)

        let correction = HealthAdjustmentDraft(
            kind: .delta,
            value: "-12.5",
            rationale: "  New damage.  ",
            evidenceSummary: "  Three leaves compared.  "
        ).request(idempotencyKey: key)
        #expect(correction?.value == -12.5)
        #expect(correction?.rationale == "New damage.")
        #expect(correction?.evidence.summary == "Three leaves compared.")
        #expect(correction?.idempotencyKey == key)
    }

    @Test("Zero means dead or unrecoverable without silently archiving")
    func zeroCopyIsExplicit() {
        let presentation = HealthPresentation(score: 0)

        #expect(presentation.title == "Health evidence: 0 out of 100")
        #expect(presentation.accessibilityDescription.contains("dead or unrecoverable"))
        #expect(presentation.accessibilityDescription.contains("separate confirmation"))
    }
}
