import Foundation

/// Behind a seam so tests can drive the screens without a server.
protocol DiagnosisService: Sendable {
    func open(slug: String, photoID: UUID?, prompt: String) async throws -> DiagnosisTurn
    func follow(slug: String, conversationID: UUID, message: String) async throws -> DiagnosisTurn
}

/// Calls POST /v1/plants/{slug}/diagnosis, which the service now implements.
struct RemoteDiagnosisService: DiagnosisService {
    let configuration: PlantyConfiguration
    let session: URLSession

    init(configuration: PlantyConfiguration, session: URLSession = .plantyDefault) {
        self.configuration = configuration
        self.session = session
    }

    func open(slug: String, photoID: UUID?, prompt: String) async throws -> DiagnosisTurn {
        try await post(
            path: "/v1/plants/\(slug)/diagnosis",
            body: DiagnosisRequest(photoID: photoID, message: prompt, conversationID: nil)
        )
    }

    func follow(slug: String, conversationID: UUID, message: String) async throws -> DiagnosisTurn {
        try await post(
            path: "/v1/plants/\(slug)/diagnosis",
            body: DiagnosisRequest(photoID: nil, message: message, conversationID: conversationID)
        )
    }

    private func post(path: String, body: DiagnosisRequest) async throws -> DiagnosisTurn {
        guard let baseURL = configuration.baseURL else { throw PlantyError.notConfigured }
        var request = URLRequest(url: baseURL.appendingPathComponent(path))
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        if let token = configuration.token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        request.httpBody = try PlantyCoders.encoder().encode(body)

        do {
            let (data, response) = try await session.data(for: request)
            guard let http = response as? HTTPURLResponse,
                  (200..<300).contains(http.statusCode)
            else {
                let status = (response as? HTTPURLResponse)?.statusCode ?? 0
                throw PlantyError.server(status: status, message: nil)
            }
            return try PlantyCoders.decoder().decode(DiagnosisTurn.self, from: data)
        } catch {
            throw PlantyError.from(error)
        }
    }
}

struct DiagnosisRequest: Encodable, Sendable {
    var photoID: UUID?
    var message: String
    var conversationID: UUID?

    enum CodingKeys: String, CodingKey {
        case photoID = "photo_id"
        case message
        case conversationID = "conversation_id"
    }
}

/// Used until the endpoint exists. It answers with the design's mock copy and
/// says so on screen, so nobody mistakes a canned reply for a real reading.
struct StubDiagnosisService: DiagnosisService {
    static let disclaimer = "Sample answer. The diagnosis service is not connected yet."

    func open(slug: String, photoID: UUID?, prompt: String) async throws -> DiagnosisTurn {
        try await Task.sleep(for: .milliseconds(700))
        return DiagnosisTurn(
            severity: .finding,
            observed: "The lower yellowing has spread since July 20.",
            interpretation: """
                Most likely this plant has been staying wet too long. \
                The pattern fits overwatering better than low light.
                """,
            actionToday: "Today: do not water it. Empty any water sitting in the tray.",
            followUpPlan: "I will compare another photo in 3 days.",
            citation: "Based on 7 photos and 14 days of moisture readings.",
            suggestedFollowUps: [
                "Show me what changed",
                "Could this be pests?",
                "What would make this urgent?"
            ],
            offeredActions: [
                DiagnosisOfferedAction(
                    title: "I emptied the tray",
                    recordsKind: .note,
                    note: "Emptied the saucer after a diagnosis."
                )
            ]
        )
    }

    func follow(slug: String, conversationID: UUID, message: String) async throws -> DiagnosisTurn {
        try await Task.sleep(for: .milliseconds(500))
        return DiagnosisTurn(
            severity: .noConcern,
            observed: "I do not see a new problem.",
            interpretation: """
                The pale patch is already visible in the earlier photo and has \
                not spread.
                """,
            actionToday: "Do not change the care plan today.",
            followUpPlan: "If it changes, take another photo from the same side.",
            citation: nil,
            suggestedFollowUps: ["Show me what changed", "What would make this urgent?"]
        )
    }
}
