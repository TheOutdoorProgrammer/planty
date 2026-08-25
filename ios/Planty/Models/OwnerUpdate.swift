import Foundation

struct PushDeviceRegistration: Codable, Sendable, Hashable {
    let token: String
    let environment: String
    let installationID: UUID

    enum CodingKeys: String, CodingKey {
        case token
        case environment
        case installationID = "installation_id"
    }
}

struct PushRegistrationReceipt: Codable, Sendable, Hashable {
    let environment: String
    let installationID: UUID
    let acceptedAt: Date

    enum CodingKeys: String, CodingKey {
        case environment
        case installationID = "installation_id"
        case acceptedAt = "accepted_at"
    }
}

struct PushServerStatus: Codable, Sendable, Hashable {
    let configured: Bool
    let environment: String
    let bundleID: String

    enum CodingKeys: String, CodingKey {
        case configured
        case environment
        case bundleID = "bundle_id"
    }
}

struct PushHealth: Codable, Sendable, Hashable {
    let server: PushServerStatus
    let registration: PushRegistrationReceipt?
}

struct PushInstallationRequest: Codable, Sendable, Hashable {
    let installationID: UUID
    let environment: String

    enum CodingKeys: String, CodingKey {
        case installationID = "installation_id"
        case environment
    }
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
