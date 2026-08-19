import Foundation

/// A server that learns a new enum case must not blank the whole list, so every
/// wire enum decodes an unrecognised value into an explicit unknown.
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

enum PlantDomain: String, FallbackDecodable, CaseIterable {
    case houseplant
    case edibleIndoor = "edible_indoor"
    case edibleOutdoor = "edible_outdoor"
    case unknown

    static let fallback = PlantDomain.unknown

    var label: String {
        switch self {
        case .houseplant: "Houseplant"
        case .edibleIndoor: "Indoor edible"
        case .edibleOutdoor: "Outdoor edible"
        case .unknown: "Unclassified"
        }
    }
}

enum PlantStatus: String, FallbackDecodable, CaseIterable {
    case alive
    case struggling
    case dormant
    case dead
    case gone
    case unknown

    static let fallback = PlantStatus.unknown

    /// Archived plants stay in the record but drop out of the daily rounds.
    var isRetired: Bool { self == .dead || self == .gone }
}

enum WateringMethod: String, FallbackDecodable, CaseIterable {
    case letpot
    case hand
    case unknown

    static let fallback = WateringMethod.unknown

    var label: String {
        switch self {
        case .letpot: "Automatic"
        case .hand: "By hand"
        case .unknown: "Unrecorded"
        }
    }
}

enum PlantAccessibility: String, FallbackDecodable, CaseIterable {
    case easy
    case awkward
    case hard
    case unknown

    static let fallback = PlantAccessibility.unknown
}

enum LightExposure: String, FallbackDecodable, CaseIterable {
    case direct
    case brightIndirect = "bright_indirect"
    case medium
    case low
    case unknown

    static let fallback = LightExposure.unknown

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

enum VerdictAction: String, FallbackDecodable, CaseIterable {
    case none
    case water
    case check
    case urgent
    case harvest
    case unknown

    static let fallback = VerdictAction.unknown

    var needsAction: Bool { self != .none }
}

enum ObservationKind: String, FallbackDecodable, CaseIterable {
    case watered
    // Not watering: it wets leaves rather than soil, no probe ever sees it,
    // and the two are scheduled on completely different rhythms.
    case misted
    case repotted
    case fertilized
    case pruned
    case harvested
    case moved
    case symptom
    case note
    case died
    case unknown

    static let fallback = ObservationKind.unknown

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

/// Whether a person, an agent or an automation recorded something. First
/// question after a mistake is always "did I do that, or did Planty".
enum ObservationSource: String, FallbackDecodable, CaseIterable {
    case app
    case agent
    case automation
    case unknown

    static let fallback = ObservationSource.unknown
}

enum SensorRole: String, FallbackDecodable, CaseIterable {
    case soilMoisture = "soil_moisture"
    case ambientTemp = "ambient_temp"
    case ambientHumidity = "ambient_humidity"
    case illuminance
    case unknown

    static let fallback = SensorRole.unknown

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
