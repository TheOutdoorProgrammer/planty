import Foundation
import Testing

@testable import Planty

@Suite("Away-period HTTP client", .serialized)
struct AwayClientTests {
    @Test("Away coverage lists from the management route")
    func listsAwayPeriods() async throws {
        let stub = IsolatedStubTransport()
        let id = UUID()
        stub.respond(json: """
            {"away_periods":[{
              "id":"\(id.uuidString)",
              "starts_at":"2103-03-01T12:00:00Z",
              "ends_at":"2103-03-05T12:00:00Z",
              "backup_contact":"Sam",
              "created_at":"2026-08-20T00:00:00Z"
            }],"count":1}
            """)

        let periods = try await stub.client().awayPeriods()
        let request = try #require(stub.requests.first)

        #expect(request.httpMethod == "GET")
        #expect(request.url?.path == "/v1/away")
        #expect(periods.map(\.id) == [id])
        #expect(periods.first?.backupContact == "Sam")
    }

    @Test("Editing sends every field so blank values clear old coverage metadata")
    func editsAwayPeriod() async throws {
        let stub = IsolatedStubTransport()
        let id = UUID()
        stub.respond(json: """
            {
              "id":"\(id.uuidString)",
              "starts_at":"2103-04-01T12:00:00Z",
              "ends_at":"2103-04-05T12:00:00Z",
              "created_at":"2026-08-20T00:00:00Z"
            }
            """)
        let formatter = ISO8601DateFormatter()
        let starts = try #require(formatter.date(from: "2103-04-01T12:00:00Z"))
        let ends = try #require(formatter.date(from: "2103-04-05T12:00:00Z"))

        _ = try await stub.client().updateAway(
            id: id,
            draft: NewAwayPeriod(startsAt: starts, endsAt: ends)
        )

        let request = try #require(stub.requests.first)
        #expect(request.httpMethod == "PATCH")
        #expect(request.url?.path == "/v1/away/\(id.uuidString)")
        let data = try #require(request.stubbedBody)
        let body = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
        #expect(body["backup_contact"] as? String == "")
        #expect(body["backup_notify"] as? String == "")
        #expect(body["note"] as? String == "")
    }

    @Test("Cancelling uses DELETE on the period id")
    func cancelsAwayPeriod() async throws {
        let stub = IsolatedStubTransport()
        let id = UUID()
        stub.respond(status: 204, json: "")

        try await stub.client().cancelAway(id: id)

        let request = try #require(stub.requests.first)
        #expect(request.httpMethod == "DELETE")
        #expect(request.url?.path == "/v1/away/\(id.uuidString)")
    }
}
