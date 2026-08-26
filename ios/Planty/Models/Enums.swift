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
extension AIJob {
    var label: String {
        switch self {
        case .assess: "Daily verdict"
        case .identify: "Identifying a plant"
        case .consult: "Asking about a plant"
        case .ask: "Asking about anything"
        case .postmortem: "Working out what killed one"
        case .ownerUpdate: "Writing to an owner"
        case .unknown: "Something new"
        }
    }

    /// What the default is, named rather than left as a bare word, because
    /// "Default" alone does not tell you what will answer.
    var defaultDetail: String {
        "Whatever the service is configured to use, normally Claude."
    }

    var explanation: String {
        switch self {
        case .assess: "Runs once a day for every plant, can control an assigned fan when the evidence supports it, and accounts for nearly all usage."
        case .identify: "Reads a photograph and names the plant. A wrong name here gets a plant watered wrongly."
        case .consult: "Answers questions about one plant, and can look things up and record what it finds."
        case .ask: "Answers questions about a plant you do not own yet, and can look things up."
        case .postmortem: "Runs once, when a plant dies, and the answer is the whole point of keeping the record."
        case .ownerUpdate: "Drafts the note that goes to whoever owns the plant."
        case .unknown: "This version of the app does not know about this job."
        }
    }
}

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
        case .airflow: "Airflow"
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
        case .airflow: "fan.fill"
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

    var careActionNoun: String {
        switch self {
        case .watered: "watering"
        case .airflow: "airflow"
        case .misted: "misting"
        case .repotted: "repotting"
        case .fertilized: "feeding"
        case .pruned: "pruning"
        case .moved: "moving"
        default: label.lowercased()
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
