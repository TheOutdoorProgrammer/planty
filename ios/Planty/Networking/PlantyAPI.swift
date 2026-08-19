import Foundation

/// The whole HTTP contract from docs/DATA-MODEL.md, as one seam. Screens talk
/// to this, never to URLSession, which is what makes the state logic testable.
protocol PlantyAPI: Sendable {
    func today() async throws -> Digest
    func plants(filter: PlantFilter) async throws -> [Plant]
    func plant(slug: String) async throws -> PlantDetail
    func createPlant(_ draft: NewPlant) async throws -> Plant
    func updatePlant(slug: String, patch: PlantPatch) async throws -> Plant
    func archivePlant(slug: String, status: PlantStatus) async throws
    func addObservation(slug: String, observation: NewObservation) async throws -> PlantObservation
    func timeline(slug: String) async throws -> PlantTimeline
    func uploadPhoto(slug: String, jpeg: Data, caption: String?, takenAt: Date) async throws -> Photo
    func acknowledge(verdictID: UUID) async throws
    func sensors() async throws -> [SensorLink]
    func logHarvest(_ harvest: NewHarvest, on slug: String) async throws -> Harvest
    func calibrate(sensorID: UUID, to calibration: SensorCalibration) async throws -> SensorLink
    func shelter(slugs: [String], indoors: Bool) async throws -> Int
    func identify(jpeg: Data, metadata: CaptureMetadata) async throws -> [IdentificationCandidate]

    /// Ask about a plant from its record. The answer comes from what is already
    /// known; a photo only rides along when the model asked to see something.
    func ask(slug: String, question: PlantQuestion) async throws -> PlantAnswer

    /// Ask about anything, naming no plant. Creates nothing, so photographing
    /// a stranger's fern is not a reason to invent a record for it.
    func ask(_ question: ScratchQuestion) async throws -> PlantAnswer

    /// Create a plant from a photograph: identified, recorded, and the picture
    /// kept as the first frame of its story.
    func createPlantFromPhoto(_ request: PlantFromPhoto) async throws -> PlantFromPhotoResult

    func linkSensor(_ link: NewSensorLink) async throws -> SensorLink

    /// Ask what killed a plant. Only answerable once it is recorded dead.
    func postmortem(slug: String) async throws -> Postmortem

    func notes(slug: String) async throws -> [PlantNote]
    func addNote(slug: String, draft: NoteDraft) async throws -> PlantNote
    func updateNote(id: UUID, draft: NoteDraft) async throws -> PlantNote
    func deleteNote(id: UUID) async throws

    func reminders(slug: String) async throws -> [Reminder]
    func setReminder(slug: String, reminder: NewReminder) async throws -> Reminder
    func deleteReminder(slug: String, kind: ObservationKind) async throws

    func health() async throws
}

/// Query parameters GET /v1/plants accepts. Nil means "do not filter".
struct PlantFilter: Sendable, Hashable {
    var domain: PlantDomain?
    var steward: String?
    var status: PlantStatus?
    var wateringMethod: WateringMethod?
    var includeArchived = false

    static let live = PlantFilter()

    var queryItems: [URLQueryItem] {
        var items: [URLQueryItem] = []
        if let domain { items.append(URLQueryItem(name: "domain", value: domain.rawValue)) }
        if let steward { items.append(URLQueryItem(name: "steward", value: steward)) }
        if let status { items.append(URLQueryItem(name: "status", value: status.rawValue)) }
        if let wateringMethod {
            items.append(URLQueryItem(name: "watering_method", value: wateringMethod.rawValue))
        }
        if includeArchived {
            items.append(URLQueryItem(name: "include_archived", value: "true"))
        }
        return items
    }
}

/// A sparse create: the service fills in domain, status, steward, access and
/// watering method, so an app can add a plant from a photo and a name.
struct NewPlant: Codable, Sendable, Hashable {
    var commonName: String
    var botanicalName: String?
    var location: String?
    var steward: String?
    var domain: PlantDomain?
    var wateringMethod: WateringMethod?
    var accessibility: PlantAccessibility?

    init(commonName: String) {
        self.commonName = commonName
    }

    enum CodingKeys: String, CodingKey {
        case commonName = "common_name"
        case botanicalName = "botanical_name"
        case location
        case steward
        case domain
        case wateringMethod = "watering_method"
        case accessibility
    }
}

/// PATCH /v1/plants/{slug}. Only what changed goes on the wire.
struct PlantPatch: Codable, Sendable, Hashable {
    var commonName: String?
    var botanicalName: String?
    var location: String?
    var status: PlantStatus?
    var steward: String?
    var lightExposure: LightExposure?
    var minTempF: Double?
    var wateringMethod: WateringMethod?
    var accessibility: PlantAccessibility?
    var haArea: String?
    var potSizeIn: Double?
    var potMaterial: String?
    var hasDrainage: Bool?

    init() {}

    var isEmpty: Bool { self == PlantPatch() }

    enum CodingKeys: String, CodingKey {
        case commonName = "common_name"
        case botanicalName = "botanical_name"
        case location
        case status
        case steward
        case lightExposure = "light_exposure"
        case minTempF = "min_temp_f"
        case wateringMethod = "watering_method"
        case accessibility
        case haArea = "ha_area"
        case potSizeIn = "pot_size_in"
        case potMaterial = "pot_material"
        case hasDrainage = "has_drainage"
    }
}

/// POST /v1/plants/from-photo. The photograph names the plant, creates it, and
/// stays as the first frame of its timeline.
struct PlantFromPhoto: Sendable {
    var jpeg: Data
    var metadata: CaptureMetadata

    /// Overrides. A given name wins outright: somebody holding the plant knows
    /// better than a model holding a picture of it.
    var commonName: String?
    var location: String?
    var steward: String?
}

/// What came back: the plant, and what else the model considered.
struct PlantFromPhotoResult: Sendable {
    let plant: Plant
    let candidates: [IdentificationCandidate]
    let photoError: String?
}

/// POST /v1/sensors. Links a Home Assistant entity to a plant, or to a zone
/// when plantID is nil. Calibration comes after, and until it does the link
/// reports without being allowed to drive anything.
struct NewSensorLink: Codable, Sendable, Hashable {
    var plantID: UUID?
    var zone: String?
    var haEntityID: String
    var role: SensorRole

    enum CodingKeys: String, CodingKey {
        case plantID = "plant_id"
        case zone
        case haEntityID = "ha_entity_id"
        case role
    }
}
