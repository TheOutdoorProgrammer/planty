import Foundation

/// One image of a plant. Bytes live in object storage; this is the pointer.
struct Photo: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let plantID: UUID

    let storageKey: String
    let takenAt: Date
    var caption: String?

    /// Kept apart from anything a human wrote, so a wrong machine reading is
    /// never later mistaken for a first-hand observation.
    var visionFindings: String?
    var analyzedAt: Date?

    let createdAt: Date
    var inheritedFrom: HistorySource?

    /// A presigned link the timeline mints, good for about half an hour. It is
    /// absent on a photo that came from anywhere else, so it cannot be stored.
    var url: URL?

    enum CodingKeys: String, CodingKey {
        case id
        case plantID = "plant_id"
        case storageKey = "storage_key"
        case takenAt = "taken_at"
        case caption
        case visionFindings = "vision_findings"
        case analyzedAt = "analyzed_at"
        case createdAt = "created_at"
        case inheritedFrom = "inherited_from"
        case url
    }

    var isAnalyzed: Bool { analyzedAt != nil }
    var renderableURL: URL? { url?.validatedRemoteImageURL }

    /// Photo labels have to carry date and finding: "Image, August 18" is not
    /// enough for anybody navigating by VoiceOver.
    func accessibilityDescription(plantName: String) -> String {
        let when = takenAt.formatted(.dateTime.month(.wide).day())
        if let finding = visionFindings, !finding.isEmpty {
            return "Photo of \(plantName) on \(when). \(finding)"
        }
        if let caption, !caption.isEmpty {
            return "Photo of \(plantName) on \(when). \(caption)"
        }
        return "Photo of \(plantName) on \(when). No description recorded yet."
    }
}

extension URL {
    var validatedRemoteImageURL: URL? {
        guard let scheme = scheme?.lowercased(), ["http", "https"].contains(scheme), host?.isEmpty == false else {
            return nil
        }
        return self
    }
}
