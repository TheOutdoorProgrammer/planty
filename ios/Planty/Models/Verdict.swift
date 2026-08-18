import Foundation

/// One plant's judgment for one day. Mirrors internal/plant.Verdict.
struct Verdict: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let plantID: UUID
    let forDate: Date

    let action: VerdictAction
    let reasoning: String
    let confidence: Double

    /// Never optional in spirit: without it a wrong verdict cannot be debugged
    /// and a right one cannot be trusted.
    let evidence: Evidence

    let createdAt: Date
    var acknowledgedAt: Date?

    enum CodingKeys: String, CodingKey {
        case id
        case plantID = "plant_id"
        case forDate = "for_date"
        case action
        case reasoning
        case confidence
        case evidence
        case createdAt = "created_at"
        case acknowledgedAt = "acknowledged_at"
    }

    var needsAction: Bool { action.needsAction }
    var isAcknowledged: Bool { acknowledgedAt != nil }
}

/// What the verdict was built from. Rendered under "Why Planty thinks this".
struct Evidence: Codable, Sendable, Hashable {
    var readingIDs: [UUID]?
    var observationIDs: [UUID]?
    var photoIDs: [UUID]?
    var sensorSummary: String?
    var modelVersion: String?

    enum CodingKeys: String, CodingKey {
        case readingIDs = "reading_ids"
        case observationIDs = "observation_ids"
        case photoIDs = "photo_ids"
        case sensorSummary = "sensor_summary"
        case modelVersion = "model_version"
    }

    var isEmpty: Bool {
        (readingIDs?.isEmpty ?? true)
            && (observationIDs?.isEmpty ?? true)
            && (photoIDs?.isEmpty ?? true)
            && (sensorSummary?.isEmpty ?? true)
    }

    var citationLine: String? {
        var parts: [String] = []
        if let count = photoIDs?.count, count > 0 {
            parts.append(count == 1 ? "1 photo" : "\(count) photos")
        }
        if let count = readingIDs?.count, count > 0 {
            parts.append(count == 1 ? "1 sensor reading" : "\(count) sensor readings")
        }
        if let count = observationIDs?.count, count > 0 {
            parts.append(count == 1 ? "1 note you wrote" : "\(count) notes you wrote")
        }
        guard !parts.isEmpty else { return nil }
        return "Based on " + parts.formatted(.list(type: .and)) + "."
    }
}
