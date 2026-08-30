import Foundation

/// Sanitized metadata for one Home Assistant entity. Home Assistant's token
/// stays on the Planty service; the phone receives only what a picker needs.
struct HomeAssistantEntity: Codable, Sendable, Hashable, Identifiable {
    let entityID: String
    let friendlyName: String
    let domain: String
    let deviceClass: String?
    let available: Bool
    let area: String?

    var id: String { entityID }

    enum CodingKeys: String, CodingKey {
        case entityID = "entity_id"
        case friendlyName = "friendly_name"
        case domain
        case deviceClass = "device_class"
        case available
        case area
    }

    var metadataLabel: String? {
        let parts = [area, deviceClass?.replacingOccurrences(of: "_", with: " ").capitalized]
            .compactMap { value -> String? in
                guard let value, !value.isEmpty else { return nil }
                return value
            }
        return parts.isEmpty ? nil : parts.joined(separator: " • ")
    }

    func matches(search query: String) -> Bool {
        let query = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !query.isEmpty else { return true }
        return [entityID, friendlyName, domain, deviceClass ?? "", area ?? ""]
            .contains { $0.localizedCaseInsensitiveContains(query) }
    }

    func isLikely(for role: SensorRole) -> Bool {
        let deviceClass = deviceClass?.lowercased()
        let tokens = discoveryTokens
        switch role {
        case .soilMoisture:
            return deviceClass == "moisture"
                || tokens.containsAny("soil", "moisture", "wetness", "substrate", "miflora")
        case .ambientTemp:
            return deviceClass == "temperature" || tokens.containsAny("temperature", "temp")
        case .ambientHumidity:
            return deviceClass == "humidity" || tokens.containsAny("humidity", "humid")
        case .illuminance:
            return deviceClass == "illuminance"
                || tokens.containsAny("illuminance", "lux", "light", "brightness")
        case .unknown:
            return true
        }
    }

    private var discoveryTokens: Set<String> {
        let text = [entityID, friendlyName, deviceClass ?? ""].joined(separator: " ")
        let components = text.lowercased().components(separatedBy: CharacterSet.alphanumerics.inverted)
        return Set(components.filter { !$0.isEmpty })
    }
}

private extension Set where Element == String {
    func containsAny(_ candidates: String...) -> Bool { candidates.contains(where: contains) }
}

extension SensorRole {
    var requiresCalibration: Bool { self == .soilMoisture }
}

struct HomeAssistantEntityListResponse: Decodable, Sendable {
    let entities: [HomeAssistantEntity]
}

enum HomeAssistantEntityFilter {
    static func all(in entities: [HomeAssistantEntity], matching query: String = "") -> [HomeAssistantEntity] {
        ordered(entities.filter { $0.matches(search: query) })
    }

    static func likely(
        in entities: [HomeAssistantEntity],
        for role: SensorRole,
        matching query: String = ""
    ) -> [HomeAssistantEntity] {
        all(in: entities, matching: query).filter { $0.isLikely(for: role) }
    }

    private static func ordered(_ entities: [HomeAssistantEntity]) -> [HomeAssistantEntity] {
        entities.sorted { left, right in
            if left.available != right.available { return left.available }
            let nameOrder = left.friendlyName.localizedCaseInsensitiveCompare(right.friendlyName)
            if nameOrder != .orderedSame { return nameOrder == .orderedAscending }
            return left.entityID.localizedCaseInsensitiveCompare(right.entityID) == .orderedAscending
        }
    }
}

/// A Home Assistant entity tied to a plant, or to a zone when plantID is nil.
/// Soil calibration follows the probe when its plant changes.
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

    var isCalibrated: Bool {
        guard let dry = dryBaseline, let wet = wetBaseline else { return false }
        return wet > dry
    }

    func fraction(of raw: Double) -> Double? {
        guard let dry = dryBaseline, let wet = wetBaseline, wet > dry else { return nil }
        return min(max((raw - dry) / (wet - dry), 0), 1)
    }
}

struct SensorCalibration: Codable, Sendable, Hashable {
    var dryBaseline: Double
    var wetBaseline: Double

    enum CodingKeys: String, CodingKey {
        case dryBaseline = "dry_baseline"
        case wetBaseline = "wet_baseline"
    }

    var readsTheRightWayRound: Bool { wetBaseline > dryBaseline }

    init?(dry: String, wet: String) {
        guard let dryValue = Double(dry.trimmingCharacters(in: .whitespaces)),
              let wetValue = Double(wet.trimmingCharacters(in: .whitespaces))
        else { return nil }
        self.init(dryBaseline: dryValue, wetBaseline: wetValue)
    }

    init(dryBaseline: Double, wetBaseline: Double) {
        self.dryBaseline = dryBaseline
        self.wetBaseline = wetBaseline
    }
}

struct SensorCalibrationDraft: Equatable {
    var dry: String
    var wet: String

    init(link: SensorLink) {
        dry = Self.inputValue(link.dryBaseline)
        wet = Self.inputValue(link.wetBaseline)
    }

    var proposed: SensorCalibration? {
        SensorCalibration(dry: dry, wet: wet)
    }

    private static func inputValue(_ value: Double?) -> String {
        guard let value else { return "" }
        return value == value.rounded() ? String(Int(value)) : String(value)
    }
}

struct CalibrationProposal: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let sensorLinkID: UUID
    let plantID: UUID
    let readingID: UUID
    let actualValue: Double
    let unit: String?
    let currentDry: Double
    let currentWet: Double
    let proposedDry: Double
    let proposedWet: Double
    let currentRelative: Double
    let proposedRelative: Double
    let reason: String
    let modelVersion: String?
    let status: String
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case sensorLinkID = "sensor_link_id"
        case plantID = "plant_id"
        case readingID = "reading_id"
        case actualValue = "actual_value"
        case unit
        case currentDry = "current_dry"
        case currentWet = "current_wet"
        case proposedDry = "proposed_dry"
        case proposedWet = "proposed_wet"
        case currentRelative = "current_relative"
        case proposedRelative = "proposed_relative"
        case reason
        case modelVersion = "model_version"
        case status
        case createdAt = "created_at"
    }
}

struct CalibrationProposalResolution: Encodable, Sendable {
    let actor: String
}

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

    var displayValue: String {
        let number = value.formatted(.number.precision(.fractionLength(0...2)))
        guard let unit = unit?.trimmingCharacters(in: .whitespacesAndNewlines), !unit.isEmpty else {
            return number
        }
        return ["%", "°F", "°C"].contains(unit) ? number + unit : number + " " + unit
    }
}

struct SensorSeries: Sendable, Hashable, Identifiable {
    let link: SensorLink
    let readings: [Reading]

    var id: UUID { link.id }

    var latest: Reading? { readings.max { $0.takenAt < $1.takenAt } }

    var span: ClosedRange<Double>? {
        guard let low = readings.map(\.value).min(), let high = readings.map(\.value).max() else { return nil }
        return low == high ? (low - 1)...(high + 1) : low...high
    }
}
