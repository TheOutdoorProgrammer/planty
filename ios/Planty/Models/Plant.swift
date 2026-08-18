import Foundation

/// One growing thing in one place. Mirrors internal/plant.Plant on the wire.
struct Plant: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let slug: String

    var commonName: String
    var botanicalName: String?
    var variety: String?

    var domain: PlantDomain
    var steward: String
    var status: PlantStatus

    var location: String
    var haArea: String?

    var accessibility: PlantAccessibility
    var wateringMethod: WateringMethod
    var letpotDripper: Int?

    var potSizeIn: Double?
    var potMaterial: String?
    var hasDrainage: Bool?
    var soilMix: String?
    var lightExposure: LightExposure?

    var minTempF: Double?
    var careProfile: CareProfile

    var acquiredAt: Date?
    var archivedAt: Date?
    var createdAt: Date
    var updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case slug
        case commonName = "common_name"
        case botanicalName = "botanical_name"
        case variety
        case domain
        case steward
        case status
        case location
        case haArea = "ha_area"
        case accessibility
        case wateringMethod = "watering_method"
        case letpotDripper = "letpot_dripper"
        case potSizeIn = "pot_size_in"
        case potMaterial = "pot_material"
        case hasDrainage = "has_drainage"
        case soilMix = "soil_mix"
        case lightExposure = "light_exposure"
        case minTempF = "min_temp_f"
        case careProfile = "care_profile"
        case acquiredAt = "acquired_at"
        case archivedAt = "archived_at"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

extension Plant {
    static let stewardSelf = "self"

    /// Somebody else owns this. Drives the purple badge and the sort, and
    /// nothing else: ownership is metadata, never severity.
    var isFriends: Bool { !steward.isEmpty && steward != Self.stewardSelf }

    var ownerName: String? { isFriends ? steward : nil }

    /// Friend plants read as "Maya's plant"; mine only says so when ambiguous.
    var ownershipLabel: String {
        isFriends ? "\(steward)'s plant" : "Mine"
    }

    var ownershipAccessibilityLabel: String {
        isFriends ? "Friend's plant, belongs to \(steward)" : "My plant"
    }

    /// A due action here depends on a human remembering it happened.
    var needsChasing: Bool { wateringMethod == .hand }

    /// Likelihood of neglect, not current thirst. Mirrors Plant.Risk() in Go so
    /// the app orders a list the same way the service does.
    var risk: Int {
        var risk = 0
        if needsChasing { risk += 2 }
        switch accessibility {
        case .hard: risk += 2
        case .awkward: risk += 1
        default: break
        }
        if isFriends { risk += 2 }
        if status == .struggling { risk += 1 }
        return risk
    }

    var displaySpecies: String? {
        if let variety, let botanicalName {
            return "\(botanicalName) '\(variety)'"
        }
        return botanicalName ?? variety
    }
}

/// Descriptive care knowledge. Every field optional: most are unknown when a
/// plant is first written down, and a prompt reads this, not a WHERE clause.
struct CareProfile: Codable, Sendable, Hashable {
    var notes: String?
    var ownerSays: String?
    var quirks: String?
    var wantsLight: LightExposure?

    var moistureLow: Double?
    var moistureHigh: Double?
    var humidityPref: String?
    var dormancy: String?

    var daysToMaturity: Int?
    var sownAt: Date?
    var transplantedAt: Date?
    var feedSchedule: String?
    var expectedHarvest: Date?
    var needsPollination: Bool?

    var hardinessZone: String?
    var frostSensitive: Bool?
    var successionDays: Int?
    var lastFrostExpected: Date?
    var firstFrostExpected: Date?

    enum CodingKeys: String, CodingKey {
        case notes
        case ownerSays = "owner_says"
        case quirks
        case wantsLight = "wants_light"
        case moistureLow = "moisture_low"
        case moistureHigh = "moisture_high"
        case humidityPref = "humidity_pref"
        case dormancy
        case daysToMaturity = "days_to_maturity"
        case sownAt = "sown_at"
        case transplantedAt = "transplanted_at"
        case feedSchedule = "feed_schedule"
        case expectedHarvest = "expected_harvest"
        case needsPollination = "needs_pollination"
        case hardinessZone = "hardiness_zone"
        case frostSensitive = "frost_sensitive"
        case successionDays = "succession_days"
        case lastFrostExpected = "last_frost_expected"
        case firstFrostExpected = "first_frost_expected"
    }
}
