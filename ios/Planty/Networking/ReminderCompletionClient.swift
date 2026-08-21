import Foundation

/// Capability kept separate from the broad app API so older/lightweight test
/// doubles can continue to model Planty. Production uses the idempotent server
/// endpoint; Today falls back to a plain observation only for such test doubles.
protocol ReminderCompleting: Sendable {
    func completeReminder(
        reminderID: UUID,
        dueAt: Date,
        idempotencyKey: UUID
    ) async throws -> PlantObservation
}

private struct ReminderCompletionRequest: Encodable, Sendable {
    let idempotencyKey: UUID
    let dueAt: Date

    enum CodingKeys: String, CodingKey {
        case idempotencyKey = "idempotency_key"
        case dueAt = "due_at"
    }
}

extension PlantyClient: ReminderCompleting {
    func completeReminder(
        reminderID: UUID,
        dueAt: Date,
        idempotencyKey: UUID
    ) async throws -> PlantObservation {
        try await send(
            "POST",
            APIPath.completeReminder(id: reminderID.uuidString),
            body: ReminderCompletionRequest(
                idempotencyKey: idempotencyKey,
                dueAt: dueAt
            )
        )
    }
}
