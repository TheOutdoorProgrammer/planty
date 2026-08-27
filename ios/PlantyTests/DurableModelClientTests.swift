import Foundation
import Testing

@testable import Planty

@Suite("Durable model HTTP client")
struct DurableModelClientTests {
    @Test("Scratch messages use their durable conversation route")
    func scratchMessageRoute() async throws {
        let transport = IsolatedStubTransport()
        let conversationID = UUID()
        let messageID = UUID()
        transport.respond(json: """
            {
              "id": "\(messageID.uuidString)",
              "conversation_id": "\(conversationID.uuidString)",
              "asked": "What is this?",
              "status": "pending",
              "created_at": "2026-08-27T19:00:00Z"
            }
            """)

        let turn = try await transport.client().enqueueScratchMessage(
            conversationID: conversationID,
            message: ConversationMessage(id: messageID, message: "What is this?", photo: nil)
        )

        let request = try #require(transport.requests.first)
        #expect(request.httpMethod == "POST")
        #expect(request.url?.path == "/v1/conversations/\(conversationID.uuidString)/messages")
        #expect(turn.id == messageID)
        #expect(turn.status == .pending)
    }

    @Test("Identification posts once and polls the same stable id")
    func identificationRoutes() async throws {
        let transport = IsolatedStubTransport()
        let id = UUID()
        let path = "/v1/identifications/\(id.uuidString)"
        transport.respond(routes: [
            path: """
                {
                  "id": "\(id.uuidString)",
                  "status": "pending"
                }
                """
        ])
        let client = transport.client()
        let metadata = CaptureMetadata(
            capturedAt: Date(timeIntervalSince1970: 1_700_000_000),
            latitude: 40.7,
            longitude: -74.0
        )

        let accepted = try await client.enqueueIdentification(
            id: id,
            jpeg: Data([0xff, 0xd8, 0xff]),
            metadata: metadata
        )
        let resumed = try await client.identification(id: id)

        #expect(accepted.id == id && accepted.status == .pending)
        #expect(resumed.id == id && resumed.status == .pending)
        #expect(transport.requests.count == 2)
        let posted = try #require(transport.requests.first)
        #expect(posted.httpMethod == "POST")
        #expect(posted.url?.path == path)
        #expect(posted.url?.query?.contains("lat=40.7") == true)
        #expect(posted.url?.query?.contains("lon=-74.0") == true)
        #expect(posted.value(forHTTPHeaderField: "Content-Type")?.hasPrefix("multipart/form-data") == true)
        #expect(transport.requests.last?.httpMethod == "GET")
        #expect(transport.requests.last?.url?.path == path)
    }
}
