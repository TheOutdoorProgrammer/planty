import Foundation

protocol ReminderResolving: Sendable {
    func resolveReminder(
        reminderID: UUID,
        dueAt: Date,
        disposition: ReminderDisposition,
        note: String,
        idempotencyKey: UUID
    ) async throws -> ReminderResolutionResult
}

struct ReminderResolutionResult: Decodable, Sendable {
    let idempotencyKey: UUID
    let reminderID: UUID
    let dueAt: Date
    let disposition: ReminderDisposition
    let note: String?
    let observation: PlantObservation?
    let respondedAt: Date

    enum CodingKeys: String, CodingKey {
        case idempotencyKey = "idempotency_key"
        case reminderID = "reminder_id"
        case dueAt = "due_at"
        case disposition
        case note
        case observation
        case respondedAt = "responded_at"
    }
}

private struct ReminderResolutionRequest: Encodable, Sendable {
    let idempotencyKey: UUID
    let dueAt: Date
    let disposition: ReminderDisposition
    let note: String

    enum CodingKeys: String, CodingKey {
        case idempotencyKey = "idempotency_key"
        case dueAt = "due_at"
        case disposition
        case note
    }
}

extension PlantyClient: ReminderResolving {
    func resolveReminder(
        reminderID: UUID,
        dueAt: Date,
        disposition: ReminderDisposition,
        note: String,
        idempotencyKey: UUID
    ) async throws -> ReminderResolutionResult {
        try await send(
            "POST",
            APIPath.resolveReminder(id: reminderID.uuidString),
            body: ReminderResolutionRequest(
                idempotencyKey: idempotencyKey,
                dueAt: dueAt,
                disposition: disposition,
                note: note
            )
        )
    }
}
