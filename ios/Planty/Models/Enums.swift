import Foundation

/// A server that learns a new enum case must not blank the whole list, so every
/// generated wire enum decodes an unrecognised value into an explicit unknown.
protocol FallbackDecodable: RawRepresentable, Codable, Sendable, Hashable
where RawValue == String {
    static var fallback: Self { get }
}

extension FallbackDecodable {
    init(from decoder: any Decoder) throws {
        let raw = try decoder.singleValueContainer().decode(String.self)
        self = Self(rawValue: raw) ?? Self.fallback
    }
}

// The enum cases themselves are generated from api/openapi.json. Everything
// below is presentation behavior and deliberately remains handwritten.
extension PlantDomain {
    var label: String {
        switch self {
        case .houseplant: "Houseplant"
        case .edibleIndoor: "Indoor edible"
        case .edibleOutdoor: "Outdoor edible"
        case .unknown: "Unclassified"
        }
    }
}

extension PlantStatus {
    /// Archived plants stay in the record but drop out of the daily rounds.
    var isRetired: Bool { self == .dead || self == .gone }

    var editLabel: String {
        switch self {
        case .alive: "Alive"
        case .struggling: "Struggling"
        case .dormant: "Dormant"
        case .dead: "Dead"
        case .gone: "Gone"
        case .unknown: "Unrecorded"
        }
    }
}

extension WateringMethod {
    var label: String {
        switch self {
        case .letpot: "Automatic"
        case .hand: "By hand"
        case .unknown: "Unrecorded"
        }
    }
}

extension PlantAccessibility {
    var editLabel: String {
        switch self {
        case .easy: "Easy to reach"
        case .awkward: "Awkward"
        case .hard: "Hard to reach"
        case .unknown: "Unrecorded"
        }
    }
}

extension LightExposure {
    var label: String {
        switch self {
        case .direct: "Direct sun"
        case .brightIndirect: "Bright indirect"
        case .medium: "Medium light"
        case .low: "Low light"
        case .unknown: "Light not recorded"
        }
    }
}

extension VerdictAction {
    var needsAction: Bool { self != .none }
}

extension ObservationKind {
    var label: String {
        switch self {
        case .watered: "Watered"
        case .misted: "Misted"
        case .repotted: "Repotted"
        case .fertilized: "Fed"
        case .pruned: "Pruned"
        case .harvested: "Harvested"
        case .moved: "Moved"
        case .symptom: "Symptom noted"
        case .note: "Note"
        case .died: "Died"
        case .unknown: "Recorded"
        }
    }

    var symbol: String {
        switch self {
        case .watered: "drop.fill"
        case .misted: "humidity.fill"
        case .repotted: "shippingbox.fill"
        case .fertilized: "sparkles"
        case .pruned: "scissors"
        case .harvested: "basket.fill"
        case .moved: "arrow.left.arrow.right"
        case .symptom: "exclamationmark.bubble.fill"
        case .note: "text.alignleft"
        case .died: "xmark.seal.fill"
        case .unknown: "questionmark.circle"
        }
    }
}

extension SensorRole {
    var label: String {
        switch self {
        case .soilMoisture: "Soil moisture"
        case .ambientTemp: "Temperature"
        case .ambientHumidity: "Humidity"
        case .illuminance: "Light"
        case .unknown: "Sensor"
        }
    }
}
