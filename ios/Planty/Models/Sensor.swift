import Foundation

/// A Home Assistant entity tied to a plant, or to a zone when plantID is nil.
/// Calibration lives here because it belongs to a probe in a pot, not a plant.
struct SensorLink: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    var plantID: UUID?
    var zone: String?

    let haEntityID: String
    let role: SensorRole

    var dryBaseline: Double?
    var wetBaseline: Double?
    var calibratedAt: Date?

    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case plantID = "plant_id"
        case zone
        case haEntityID = "ha_entity_id"
        case role
        case dryBaseline = "dry_baseline"
        case wetBaseline = "wet_baseline"
        case calibratedAt = "calibrated_at"
        case createdAt = "created_at"
    }

    /// An uncalibrated link may report but must never drive a decision: a
    /// confident wrong alert is worse than no alert.
    var isCalibrated: Bool {
        guard let dry = dryBaseline, let wet = wetBaseline else { return false }
        return wet > dry
    }

    /// Maps a raw reading onto 0...1 between this probe's own baselines. Two
    /// probes' absolute numbers are never comparable.
    func fraction(of raw: Double) -> Double? {
        guard let dry = dryBaseline, let wet = wetBaseline, wet > dry else { return nil }
        return min(max((raw - dry) / (wet - dry), 0), 1)
    }
}

/// One sample, keyed on the link because the plant a probe serves can change.
struct Reading: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let sensorLinkID: UUID
    let value: Double
    var unit: String?
    let takenAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case sensorLinkID = "sensor_link_id"
        case value
        case unit
        case takenAt = "taken_at"
    }
}

/// A link plus its recent samples, which is what the evidence disclosure draws.
struct SensorSeries: Sendable, Hashable, Identifiable {
    let link: SensorLink
    let readings: [Reading]

    var id: UUID { link.id }

    var latest: Reading? {
        readings.max { $0.takenAt < $1.takenAt }
    }

    var span: ClosedRange<Double>? {
        guard let low = readings.map(\.value).min(),
              let high = readings.map(\.value).max()
        else { return nil }
        return low == high ? (low - 1)...(high + 1) : low...high
    }
}
