import Foundation
import Testing

@testable import Planty

@Suite("The HTTP client", .serialized)
struct PlantyClientTests {
    @Test("The bearer token and path land on the request")
    func requestShape() async throws {
        StubTransport.respond(json: #"{"plants": [], "count": 0}"#)
        _ = try await StubTransport.client().plants(filter: .live)

        let request = try #require(StubResponder.shared.requests.first)
        #expect(request.url?.path == "/v1/plants")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer s3cret")
        #expect(request.httpMethod == "GET")
    }

    @Test("No token configured means no Authorization header, not an empty one")
    func noToken() async throws {
        StubTransport.respond(json: #"{"plants": [], "count": 0}"#)
        _ = try await StubTransport.client(token: nil).plants(filter: .live)

        let request = try #require(StubResponder.shared.requests.first)
        #expect(request.value(forHTTPHeaderField: "Authorization") == nil)
    }

    @Test("Filters become query items the service understands")
    func filterQuery() async throws {
        StubTransport.respond(json: #"{"plants": []}"#)
        var filter = PlantFilter()
        filter.domain = .edibleIndoor
        filter.steward = "Maya"
        filter.includeArchived = true
        _ = try await StubTransport.client().plants(filter: filter)

        let url = try #require(StubResponder.shared.requests.first?.url?.absoluteString)
        #expect(url.contains("domain=edible_indoor"))
        #expect(url.contains("steward=Maya"))
        #expect(url.contains("include_archived=true"))
    }

    @Test("A plant list decodes out of its envelope")
    func decodesList() async throws {
        StubTransport.respond(json: #"{"plants": [\#(ModelDecodingTests.monaJSON)], "count": 1}"#)
        let plants = try await StubTransport.client().plants(filter: .live)
        #expect(plants.count == 1)
        #expect(plants.first?.commonName == "Mona")
    }

    @Test("The digest endpoint decodes")
    func decodesDigest() async throws {
        StubTransport.respond(json: ModelDecodingTests.digestJSON)
        let digest = try await StubTransport.client().today()
        #expect(digest.checked == 8)
        #expect(digest.entries.count == 1)
    }

    @Test("401 becomes unauthorized, not a generic server error")
    func unauthorized() async {
        StubTransport.respond(status: 401, json: #"{"error":"unauthorized"}"#)
        await #expect(throws: PlantyError.unauthorized) {
            _ = try await StubTransport.client().today()
        }
    }

    @Test("404 becomes notFound")
    func notFound() async {
        StubTransport.respond(status: 404, json: #"{"error":"plant not found"}"#)
        await #expect(throws: PlantyError.notFound) {
            _ = try await StubTransport.client().plant(slug: "ghost")
        }
    }

    @Test("A 500 carries the service's own message through")
    func serverMessage() async {
        StubTransport.respond(status: 500, json: #"{"error":"the daily run failed"}"#)
        await #expect(throws: PlantyError.server(status: 500, message: "the daily run failed")) {
            _ = try await StubTransport.client().today()
        }
    }

    @Test("A dropped connection is offline, and offline is transient")
    func offline() async throws {
        StubTransport.fail(with: .notConnectedToInternet)
        await #expect(throws: PlantyError.offline) {
            _ = try await StubTransport.client().today()
        }
        #expect(PlantyError.offline.isTransient)
        #expect(!PlantyError.unauthorized.isTransient)
    }

    @Test("No base URL fails before any request is made")
    func notConfigured() async {
        let client = PlantyClient(
            configuration: .unconfigured,
            session: StubTransport.session()
        )
        await #expect(throws: PlantyError.notConfigured) {
            _ = try await client.today()
        }
    }

    @Test("Malformed JSON is a decoding error, never a silent empty result")
    func malformedJSON() async {
        StubTransport.respond(json: "{ not json at all")
        await #expect(throws: (any Error).self) {
            _ = try await StubTransport.client().today()
        }
    }

    @Test("Posting an observation sends JSON on the right path")
    func postsObservation() async throws {
        let observationJSON = """
            {
              "id": "3D3C3B3A-3938-3736-3534-333231302928",
              "plant_id": "0651DE3F-6EC5-4A7B-9981-8CF8F53D0F4D",
              "kind": "watered",
              "occurred_at": "2026-08-18T09:00:00Z",
              "source": "app",
              "created_at": "2026-08-18T09:00:01Z"
            }
            """
        StubTransport.respond(status: 201, json: observationJSON)
        let saved = try await StubTransport.client().addObservation(
            slug: "mona",
            observation: NewObservation(kind: .watered)
        )

        let request = try #require(StubResponder.shared.requests.first)
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/plants/mona/observations")
        #expect(saved.kind == .watered)
        #expect(saved.source == .app)
    }

    /// The app and the Dusk plugin had both guessed a flat /v1/harvests that
    /// Planty does not serve, so every harvest logged would have 404d.
    @Test("A harvest is posted under the plant that grew it")
    func logsHarvestUnderItsPlant() async throws {
        let harvestJSON = """
            {
              "id": "\(UUID().uuidString)",
              "plant_id": "\(UUID().uuidString)",
              "occurred_at": "2026-08-18T09:00:00Z",
              "quantity": 4,
              "unit": "fruit",
              "created_at": "2026-08-18T09:00:01Z"
            }
            """
        StubTransport.respond(status: 201, json: harvestJSON)
        let saved = try await StubTransport.client().logHarvest(
            NewHarvest(occurredAt: Date(), quantity: 4, unit: "fruit"),
            on: "mona"
        )

        let request = try #require(StubResponder.shared.requests.first)
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/plants/mona/harvests")
        #expect(saved.unit == "fruit")
    }

    @Test("Calibration is a PATCH against the link, carrying both baselines")
    func calibratesASensor() async throws {
        let id = UUID()
        let sensorJSON = """
            {
              "id": "\(id.uuidString)",
              "ha_entity_id": "sensor.mona_moisture",
              "role": "soil_moisture",
              "dry_baseline": 320,
              "wet_baseline": 720,
              "calibrated_at": "2026-08-18T09:00:00Z",
              "created_at": "2026-08-01T09:00:00Z"
            }
            """
        StubTransport.respond(json: sensorJSON)
        let saved = try await StubTransport.client().calibrate(
            sensorID: id,
            to: SensorCalibration(dryBaseline: 320, wetBaseline: 720)
        )

        let request = try #require(StubResponder.shared.requests.first)
        #expect(request.httpMethod == "PATCH")
        #expect(request.url?.path == "/v1/sensors/\(id.uuidString)")
        #expect(saved.isCalibrated)
    }

    @Test("Acknowledging a verdict hits the ack path")
    func acknowledges() async throws {
        StubTransport.respond(json: #"{"ok":true}"#)
        let id = UUID()
        try await StubTransport.client().acknowledge(verdictID: id)

        let request = try #require(StubResponder.shared.requests.first)
        #expect(request.url?.path == "/v1/verdicts/\(id.uuidString)/ack")
        #expect(request.httpMethod == "POST")
    }

    @Test("Archiving is a DELETE that carries the resulting status")
    func archives() async throws {
        StubTransport.respond(json: #"{"archived":"mona"}"#)
        try await StubTransport.client().archivePlant(slug: "mona", status: .dead)

        let request = try #require(StubResponder.shared.requests.first)
        #expect(request.httpMethod == "DELETE")
        #expect(request.url?.path == "/v1/plants/mona")
        #expect(request.url?.query?.contains("status=dead") == true)
    }

    /// Every write asserts its own method and path. Two clients have already
    /// shipped a route the service does not serve, each caught by nothing.
    @Test("Every remaining write goes to the path the contract documents")
    func writesGoWhereTheyAreDocumented() async throws {
        let plantJSON = try encoded(Plant.fixture())

        StubTransport.respond(status: 201, json: plantJSON)
        _ = try await StubTransport.client().createPlant(NewPlant(commonName: "Mona"))
        var request = try #require(StubResponder.shared.requests.first)
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/plants")

        StubTransport.respond(json: plantJSON)
        _ = try await StubTransport.client().updatePlant(slug: "mona", patch: PlantPatch())
        request = try #require(StubResponder.shared.requests.first)
        #expect(request.httpMethod == "PATCH")
        #expect(request.url?.path == "/v1/plants/mona")

        StubTransport.respond(json: #"{"moved":1,"where":"indoors"}"#)
        _ = try await StubTransport.client().shelter(slugs: ["mona"], indoors: true)
        request = try #require(StubResponder.shared.requests.first)
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/shelter")

        StubTransport.respond(json: #"{"moved":1,"where":"outside"}"#)
        _ = try await StubTransport.client().shelter(slugs: ["mona"], indoors: false)
        request = try #require(StubResponder.shared.requests.first)
        #expect(request.url?.path == "/v1/unshelter")

        StubTransport.respond(status: 201, json: try encoded(photoFixture()))
        _ = try await StubTransport.client().uploadPhoto(
            slug: "mona",
            jpeg: Data([0xFF, 0xD8, 0xFF]),
            caption: nil,
            takenAt: Date()
        )
        request = try #require(StubResponder.shared.requests.first)
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/plants/mona/photos")
    }

    /// Encoding a real model beats hand-written JSON: a response the client
    /// cannot decode would fail this test for the wrong reason.
    private func encoded(_ value: some Encodable) throws -> String {
        let data = try PlantyCoders.encoder().encode(value)
        return try #require(String(bytes: data, encoding: .utf8))
    }

    private func photoFixture() -> Photo {
        Photo(
            id: UUID(),
            plantID: UUID(),
            storageKey: "plants/mona/1.jpg",
            takenAt: Date(),
            createdAt: Date()
        )
    }

    @Test("A slug with a space still builds a usable path")
    func escapesSlug() async throws {
        StubTransport.respond(json: #"{"observations": []}"#)
        _ = try? await StubTransport.client().plant(slug: "big fern")

        let url = try #require(StubResponder.shared.requests.first?.url?.absoluteString)
        #expect(!url.contains(" "))
    }

    /// The ask tests live in this suite rather than their own because every one
    /// of them drives the same global stub, and only this suite is serialized.
    private var jpeg: Data { Data([0xFF, 0xD8, 0xFF, 0xE0]) }

    @Test("A question with no plant behind it posts to /v1/ask")
    func scratchAskCarriesItsPhoto() async throws {
        StubTransport.respond(json: Self.answerJSON)
        let answer = try await StubTransport.client()
            .ask(ScratchQuestion(message: "what is this?", photo: jpeg))

        let request = try #require(StubResponder.shared.requests.first)
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/ask")
        #expect(request.timeoutInterval == Patience.model, "a scratch ask waits on a model")
        #expect(answer.reply == "That is a peace lily.")
        #expect(answer.suggestedFollowUps == ["Is it safe for cats?"])

        let body = try fields(of: request)
        #expect(body["photo"] as? String == jpeg.base64EncodedString())
        #expect(body["message"] as? String == "what is this?")
    }

    @Test("A question about a plant posts to its own path, photo and all")
    func plantAskCarriesItsPhoto() async throws {
        StubTransport.respond(json: Self.answerJSON)
        _ = try await StubTransport.client().ask(
            slug: "mona",
            question: PlantQuestion(message: "look here", photo: jpeg)
        )

        let request = try #require(StubResponder.shared.requests.first)
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/plants/mona/ask")
        #expect(request.timeoutInterval == Patience.model)
        #expect(try fields(of: request)["photo"] as? String == jpeg.base64EncodedString())
    }

    @Test("A question with no photo sends no photo key at all")
    func aQuestionWithoutAPhotoOmitsIt() async throws {
        StubTransport.respond(json: Self.answerJSON)
        _ = try await StubTransport.client().ask(ScratchQuestion(message: "what is this?"))

        let request = try #require(StubResponder.shared.requests.first)
        let body = try fields(of: request)
        #expect(body["photo"] == nil)
        #expect(body["conversation_id"] == nil, "a new conversation resumes nothing")
    }

    private func fields(of request: URLRequest) throws -> [String: Any] {
        let sent = try #require(request.stubbedBody)
        return try #require(JSONSerialization.jsonObject(with: sent) as? [String: Any])
    }

    static let answerJSON = """
        {
          "id": "4E4D4C4B-4A49-4847-4645-444342414039",
          "conversation_id": "5F5E5D5C-5B5A-5958-5756-555453525150",
          "reply": "That is a peace lily.",
          "confidence": 0.95,
          "looked_at": "",
          "suggested_follow_ups": ["Is it safe for cats?"]
        }
        """
}

@Suite("Multipart photo upload")
struct MultipartBodyTests {
    @Test("Fields and the file land in one body with a closing boundary")
    func bodyShape() throws {
        var body = MultipartBody(boundary: "TESTBOUNDARY")
        body.appendFile(
            name: "photo",
            filename: "mona.jpg",
            contentType: "image/jpeg",
            data: Data([0xFF, 0xD8, 0xFF])
        )
        body.appendField(name: "caption", value: "lower leaves")

        let text = try #require(String(bytes: body.finished(), encoding: .isoLatin1))
        #expect(text.contains("--TESTBOUNDARY\r\n"))
        #expect(text.contains(#"name="photo"; filename="mona.jpg""#))
        #expect(text.contains("Content-Type: image/jpeg"))
        #expect(text.contains(#"name="caption""#))
        #expect(text.hasSuffix("--TESTBOUNDARY--\r\n"))
        #expect(body.contentType == "multipart/form-data; boundary=TESTBOUNDARY")
    }
}

// A model call takes seven to twenty seconds before it opens a photograph or
// runs a command; holding it to the same limit as a database read is what put
// "The service did not answer in time" on screen over a working answer.
@Suite("Patience")
struct PatienceTests {
    @Test("Waiting on a model gets longer than reading a record")
    func modelCallsWaitLonger() {
        #expect(Patience.model > Patience.ordinary)
        #expect(Patience.model >= 120, "a reply that opens a photo can take a while")
    }

    @Test("An ordinary read still fails fast")
    func ordinaryCallsFailFast() {
        #expect(Patience.ordinary <= 30, "a hung read must become a visible error")
    }

    /// `timeoutIntervalForResource` belongs to the session and caps the whole
    /// load, so a 60 second one silently truncated every 180 second request.
    @Test("The patient session's resource limit does not cap its own requests")
    func patientSessionIsNotCappedByItsOwnResourceLimit() {
        let patient = URLSession.plantyPatient.configuration

        #expect(patient.timeoutIntervalForRequest >= Patience.model)
        #expect(patient.timeoutIntervalForResource >= Patience.model)
    }

    /// The client only reaches for the patient session when a caller asked for
    /// patience, so a default-session client must still hand model calls over.
    @Test("A client built the ordinary way still has a patient session behind it")
    func defaultClientKeepsAPatientSession() {
        let client = PlantyClient(configuration: .unconfigured)

        #expect(client.patientSession === URLSession.plantyPatient)
        #expect(client.session !== client.patientSession)
    }
}
