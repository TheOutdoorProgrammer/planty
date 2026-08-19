import Foundation

/// One recorded event in a plant's life. Sensor samples are Reading instead.
/// Not named Observation: that shadows the module @Observable expands against.
struct PlantObservation: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let plantID: UUID
    let kind: ObservationKind
    var body: String?

    let occurredAt: Date
    let source: ObservationSource
    var actor: String?

    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case plantID = "plant_id"
        case kind
        case body
        case occurredAt = "occurred_at"
        case source
        case actor
        case createdAt = "created_at"
    }

    /// Dry soil goes hydrophobic, so a watering claim is worth verifying
    /// against what the probe did afterwards.
    var isVerifiable: Bool { kind == .watered }
}

/// What the app posts to record something. The server fills in the rest.
struct NewObservation: Codable, Sendable, Hashable {
    var kind: ObservationKind
    var body: String?
    var occurredAt: Date
    var source: ObservationSource
    var actor: String?

    init(
        kind: ObservationKind,
        body: String? = nil,
        occurredAt: Date = Date(),
        source: ObservationSource = .app,
        actor: String? = nil
    ) {
        self.kind = kind
        self.body = body
        self.occurredAt = occurredAt
        self.source = source
        self.actor = actor
    }

    enum CodingKeys: String, CodingKey {
        case kind
        case body
        case occurredAt = "occurred_at"
        case source
        case actor
    }
}

/// Kept out of the observation log: yield per season has to aggregate.
struct Harvest: Codable, Sendable, Hashable, Identifiable {
    var slug: String? = nil
    var commonName: String? = nil
    let id: UUID
    let plantID: UUID
    let occurredAt: Date
    let quantity: Double
    let unit: String
    var notes: String?
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case slug
        case commonName = "common_name"
        case plantID = "plant_id"
        case occurredAt = "occurred_at"
        case quantity
        case unit
        case notes
        case createdAt = "created_at"
    }
}

struct HarvestListResponse: Decodable, Sendable {
    let harvests: [Harvest]
    var count: Int?
}

/// The plant comes from the path Planty is posted to, so it is deliberately
/// absent here: a second copy in the body could disagree with it.
struct NewHarvest: Codable, Sendable, Hashable {
    var occurredAt: Date
    var quantity: Double
    var unit: String
    var notes: String?

    enum CodingKeys: String, CodingKey {
        case occurredAt = "occurred_at"
        case quantity
        case unit
        case notes
    }
}
