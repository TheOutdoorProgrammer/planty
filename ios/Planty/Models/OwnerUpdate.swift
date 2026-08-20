import Foundation

struct PushDeviceRegistration: Codable, Sendable, Hashable {
    let token: String
    let environment: String
}

struct OwnerUpdateRequest: Codable, Sendable, Hashable {
    let steward: String
}

struct OwnerUpdate: Codable, Sendable, Hashable {
    let steward: String
    let summary: String
    let photos: [OwnerUpdatePhoto]
}

struct OwnerUpdatePhoto: Codable, Sendable, Hashable, Identifiable {
    let plantName: String
    let plantSlug: String
    let takenAt: Date
    let url: URL

    var id: String { plantSlug }

    enum CodingKeys: String, CodingKey {
        case plantName = "plant_name"
        case plantSlug = "plant_slug"
        case takenAt = "taken_at"
        case url
    }
}
