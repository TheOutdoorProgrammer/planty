import Foundation

/// What killed a plant, written once it is recorded dead. The point of
/// archiving rather than deleting is that this can be written at all.
struct Postmortem: Codable, Sendable, Hashable, Identifiable {
    var slug: String? = nil
    var commonName: String? = nil
    let id: UUID
    let plantID: UUID

    let likelyCause: String
    let narrative: String
    var evidence: Evidence? = nil

    /// One sentence that would change what happens next time. The reason the
    /// record was kept, and the only part worth reading twice.
    let lesson: String?
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case slug
        case commonName = "common_name"
        case plantID = "plant_id"
        case likelyCause = "likely_cause"
        case narrative
        case evidence
        case lesson
        case createdAt = "created_at"
    }
}

struct PostmortemListResponse: Decodable, Sendable {
    let postmortems: [Postmortem]
    var count: Int?
}

/// The envelope POST /v1/plants/{slug}/from-photo answers with: the plant it
/// made, what else it considered, and whether keeping the photo failed.
struct PlantFromPhotoResponse: Decodable, Sendable {
    let plant: Plant
    let candidates: [IdentificationCandidate]?
    let photoError: String?

    enum CodingKeys: String, CodingKey {
        case plant
        case candidates
        case photoError = "photo_error"
    }
}

/// For POSTs that carry nothing. The service reads no body; sending `{}` keeps
/// one code path for every write rather than a bare-request special case.
struct EmptyBody: Encodable, Sendable {}
