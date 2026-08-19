import Foundation

/// GET /v1/plants/{slug}. Everything past `plant` is optional because the
/// service is still growing into the documented response.
struct PlantDetail: Decodable, Sendable, Hashable {
    let plant: Plant
    var risk: Int?
    var observations: [PlantObservation]?
    var lastWatered: Date?
    var verdict: Verdict?
    var photos: [Photo]?
    var sensors: [SensorLink]?
    var readings: [Reading]?

    /// Read from the envelope as well as from the plant inside it, because the
    /// endpoint may hang it off either and reading only one would show nothing.
    var toxicity: Toxicity?

    enum CodingKeys: String, CodingKey {
        case plant
        case risk
        case observations
        case lastWatered = "last_watered"
        case verdict
        case photos
        case sensors
        case readings
        case toxicity
    }
}

/// GET /v1/plants/{slug}/timeline. The endpoint promises observations, photos,
/// readings and verdicts merged; the app does the merging so it can group them
/// into a story rather than render a log.
struct PlantTimeline: Decodable, Sendable, Hashable {
    var observations: [PlantObservation]
    var photos: [Photo]
    var verdicts: [Verdict]
    var sensors: [SensorLink]
    var readings: [Reading]

    enum CodingKeys: String, CodingKey {
        case observations
        case photos
        case verdicts
        case sensors
        case readings
    }

    init(
        observations: [PlantObservation] = [],
        photos: [Photo] = [],
        verdicts: [Verdict] = [],
        sensors: [SensorLink] = [],
        readings: [Reading] = []
    ) {
        self.observations = observations
        self.photos = photos
        self.verdicts = verdicts
        self.sensors = sensors
        self.readings = readings
    }

    init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        observations = try container.decodeIfPresent([PlantObservation].self, forKey: .observations) ?? []
        photos = try container.decodeIfPresent([Photo].self, forKey: .photos) ?? []
        verdicts = try container.decodeIfPresent([Verdict].self, forKey: .verdicts) ?? []
        sensors = try container.decodeIfPresent([SensorLink].self, forKey: .sensors) ?? []
        readings = try container.decodeIfPresent([Reading].self, forKey: .readings) ?? []
    }

    var isEmpty: Bool {
        observations.isEmpty && photos.isEmpty && verdicts.isEmpty
    }

    /// The timeline endpoint returns photos and nothing else; the plant
    /// endpoint returns the observations, readings and current verdict. A story
    /// built from only one of them is the photos with no reason for any of it.
    func merging(_ detail: PlantDetail) -> PlantTimeline {
        PlantTimeline(
            observations: observations.isEmpty ? detail.observations ?? [] : observations,
            photos: photos.isEmpty ? detail.photos ?? [] : photos,
            verdicts: verdicts.isEmpty ? [detail.verdict].compactMap { $0 } : verdicts,
            sensors: sensors.isEmpty ? detail.sensors ?? [] : sensors,
            readings: readings.isEmpty ? detail.readings ?? [] : readings
        )
    }

    /// Series for the "Why Planty thinks this" disclosure, newest link first.
    var series: [SensorSeries] {
        sensors.map { link in
            SensorSeries(
                link: link,
                readings: readings
                    .filter { $0.sensorLinkID == link.id }
                    .sorted { $0.takenAt < $1.takenAt }
            )
        }
        .filter { !$0.readings.isEmpty }
    }
}

/// GET /v1/plants and GET /v1/sensors both wrap their list in a named key.
struct PlantListResponse: Decodable, Sendable {
    let plants: [Plant]
    var count: Int?
}

struct SensorListResponse: Decodable, Sendable {
    let sensors: [SensorLink]
}

/// Naming plants rather than sending `all`: the app knows exactly which ones
/// were shown, and `all` from a phone is a lot of pots to be wrong about.
struct ShelterRequest: Encodable, Sendable {
    let slugs: [String]
}

struct ShelterResponse: Decodable, Sendable {
    let moved: Int
}

struct ObservationListResponse: Decodable, Sendable {
    let observations: [PlantObservation]
}
