import Foundation

/// Something written down about a plant. Many per plant, each editable alone,
/// unlike the one care-profile field the app overwrites.
struct PlantNote: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let plantID: UUID
    let title: String?
    let body: String
    let createdAt: Date
    let updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case plantID = "plant_id"
        case title
        case body
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }

    /// A second of slack, because the store stamps both on insert and they can
    /// differ by microseconds without anybody having edited anything.
    var wasEdited: Bool { updatedAt.timeIntervalSince(createdAt) > 1 }

    var heading: String? {
        guard let title, !title.trimmingCharacters(in: .whitespaces).isEmpty else { return nil }
        return title
    }
}

/// Writing or changing a note. Both fields optional on a change, so editing a
/// body cannot silently blank a title nobody mentioned.
struct NoteDraft: Encodable, Sendable, Hashable {
    var title: String?
    var body: String?
}

struct NoteListResponse: Decodable, Sendable {
    let notes: [PlantNote]
}
