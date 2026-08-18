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
        #expect(request.url?.query?.contains("status=dead") == true)
    }

    @Test("A slug with a space still builds a usable path")
    func escapesSlug() async throws {
        StubTransport.respond(json: #"{"observations": []}"#)
        _ = try? await StubTransport.client().plant(slug: "big fern")

        let url = try #require(StubResponder.shared.requests.first?.url?.absoluteString)
        #expect(!url.contains(" "))
    }
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
