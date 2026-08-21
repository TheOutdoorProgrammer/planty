import Foundation

private struct VerdictCompletionRequest: Encodable, Sendable {
    let idempotencyKey: UUID
    let kind: ObservationKind
    let body: String

    enum CodingKeys: String, CodingKey {
        case idempotencyKey = "idempotency_key"
        case kind
        case body
    }
}

extension PlantyClient {
    func completeVerdict(
        slug: String,
        verdictID: UUID,
        kind: ObservationKind,
        note: String,
        idempotencyKey: UUID
    ) async throws -> PlantObservation {
        try await send(
            "POST",
            APIPath.completeVerdict(id: verdictID.uuidString),
            body: VerdictCompletionRequest(
                idempotencyKey: idempotencyKey,
                kind: kind,
                body: note
            )
        )
    }

    func observationsPage(slug: String, cursor: String) async throws -> ObservationListResponse {
        try await get(
            APIPath.listObservations(slug: escaped(slug)),
            query: [URLQueryItem(name: "cursor", value: cursor)]
        )
    }

    func timelinePage(slug: String, cursor: String) async throws -> PlantTimeline {
        try await get(
            APIPath.getTimeline(slug: escaped(slug)),
            query: [URLQueryItem(name: "cursor", value: cursor)]
        )
    }
}
