import Foundation

/// The app-facing service seam. Raw HTTP paths and closed wire enums come from
/// api/openapi.json; screens talk to this protocol rather than URLSession so
/// state logic stays testable.
protocol PlantyAPI: Sendable {
    func today() async throws -> Digest
    func plants(filter: PlantFilter) async throws -> [Plant]
    func plant(slug: String) async throws -> PlantDetail
    func createPlant(_ draft: NewPlant) async throws -> Plant
    func updatePlant(slug: String, patch: PlantPatch) async throws -> Plant
    func archivePlant(slug: String, status: PlantStatus) async throws
    func addObservation(slug: String, observation: NewObservation) async throws -> PlantObservation
    func completeVerdict(
        slug: String,
        verdictID: UUID,
        kind: ObservationKind,
        note: String,
        idempotencyKey: UUID
    ) async throws -> PlantObservation
    func observationsPage(slug: String, cursor: String) async throws -> ObservationListResponse
    func timeline(slug: String) async throws -> PlantTimeline
    func timelinePage(slug: String, cursor: String) async throws -> PlantTimeline
    func uploadPhoto(slug: String, jpeg: Data, caption: String?, takenAt: Date) async throws -> Photo
    func acknowledge(verdictID: UUID) async throws
    func sensors() async throws -> [SensorLink]
    func homeAssistantEntities() async throws -> [HomeAssistantEntity]
    func managedChoices() async throws -> ManagedChoices
    func logHarvest(_ harvest: NewHarvest, on slug: String) async throws -> Harvest
    func harvests(slug: String?) async throws -> [Harvest]
    func calibrate(sensorID: UUID, to calibration: SensorCalibration) async throws -> SensorLink
    func shelter(slugs: [String], indoors: Bool) async throws -> Int
    func identify(jpeg: Data, metadata: CaptureMetadata) async throws -> [IdentificationCandidate]
    func ask(slug: String, question: PlantQuestion) async throws -> PlantAnswer
    func ask(_ question: ScratchQuestion) async throws -> PlantAnswer
    func createPlantFromPhoto(_ request: PlantFromPhoto) async throws -> PlantFromPhotoResult
    func linkSensor(_ link: NewSensorLink) async throws -> SensorLink
    func postmortem(slug: String) async throws -> Postmortem
    func postmortems() async throws -> [Postmortem]
    func questions(status: QuestionStatus) async throws -> [OpenQuestion]
    func createQuestion(_ draft: NewOpenQuestion) async throws -> OpenQuestion
    func planAway(_ draft: NewAwayPeriod) async throws -> AwayPeriod
    func awayPeriods() async throws -> [AwayPeriod]
    func updateAway(id: UUID, draft: NewAwayPeriod) async throws -> AwayPeriod
    func cancelAway(id: UUID) async throws
    func coldWatch(forecastLowF: Double) async throws -> ColdWatch
    func notes(slug: String) async throws -> [PlantNote]
    func answerQuestion(id: UUID, answer: String) async throws
    func householdNotes() async throws -> [PlantNote]
    func addHouseholdNote(draft: NoteDraft) async throws -> PlantNote
    func addNote(slug: String, draft: NoteDraft) async throws -> PlantNote
    func updateNote(id: UUID, draft: NoteDraft) async throws -> PlantNote
    func deleteNote(id: UUID) async throws
    func reminders(slug: String) async throws -> [Reminder]
    func setReminder(slug: String, reminder: NewReminder) async throws -> Reminder
    func deleteReminder(slug: String, kind: ObservationKind) async throws
    func registerPushDevice(_ device: PushDeviceRegistration) async throws
    func ownerUpdate(steward: String) async throws -> OwnerUpdate
    func health() async throws
}

extension PlantyAPI {
    func homeAssistantEntities() async throws -> [HomeAssistantEntity] {
        throw PlantyError.server(status: 503, message: "Home Assistant discovery is unavailable from this client.")
    }

    func managedChoices() async throws -> ManagedChoices { .empty }

    /// Production overrides this with the atomic endpoint. The fallback keeps
    /// small test doubles source-compatible while still modeling the old two
    /// operations explicitly in tests that have not opted into the new seam.
    func completeVerdict(
        slug: String,
        verdictID: UUID,
        kind: ObservationKind,
        note: String,
        idempotencyKey: UUID
    ) async throws -> PlantObservation {
        let observation = try await addObservation(
            slug: slug,
            observation: NewObservation(kind: kind, body: note.isEmpty ? nil : note)
        )
        try await acknowledge(verdictID: verdictID)
        return observation
    }

    func observationsPage(slug: String, cursor: String) async throws -> ObservationListResponse {
        throw PlantyError.server(status: 503, message: "Observation pagination is unavailable from this client.")
    }

    func timelinePage(slug: String, cursor: String) async throws -> PlantTimeline {
        throw PlantyError.server(status: 503, message: "Photo pagination is unavailable from this client.")
    }

    // Defaults keep lightweight test doubles source-compatible. Production
    // PlantyClient implements these and the feature surfaces handle unavailable
    // endpoints as normal service errors.
    func awayPeriods() async throws -> [AwayPeriod] { [] }

    func updateAway(id: UUID, draft: NewAwayPeriod) async throws -> AwayPeriod {
        throw PlantyError.server(status: 503, message: "Away-period editing is unavailable from this client.")
    }

    func cancelAway(id: UUID) async throws {
        throw PlantyError.server(status: 503, message: "Away-period cancellation is unavailable from this client.")
    }

    func registerPushDevice(_ device: PushDeviceRegistration) async throws {
        throw PlantyError.server(status: 503, message: "Push registration is unavailable from this client.")
    }

    func ownerUpdate(steward: String) async throws -> OwnerUpdate {
        throw PlantyError.server(status: 503, message: "Owner updates are unavailable from this client.")
    }
}

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
        if let wateringMethod { items.append(URLQueryItem(name: "watering_method", value: wateringMethod.rawValue)) }
        if includeArchived { items.append(URLQueryItem(name: "include_archived", value: "true")) }
        return items
    }
}

struct NewPlant: Codable, Sendable, Hashable {
    var commonName: String
    var botanicalName: String?
    var location: String?
    var haArea: String?
    var steward: String?
    var potMaterial: String?
    var domain: PlantDomain?
    var wateringMethod: WateringMethod?
    var accessibility: PlantAccessibility?
    init(commonName: String) { self.commonName = commonName }
    enum CodingKeys: String, CodingKey {
        case commonName = "common_name"
        case botanicalName = "botanical_name"
        case location
        case haArea = "ha_area"
        case steward
        case potMaterial = "pot_material"
        case domain
        case wateringMethod = "watering_method"
        case accessibility
    }
}

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
    var toxicity: Toxicity?
    init() {}
    var isEmpty: Bool { self == PlantPatch() }
    enum CodingKeys: String, CodingKey {
        case commonName = "common_name"
        case botanicalName = "botanical_name"
        case location, status, steward
        case lightExposure = "light_exposure"
        case minTempF = "min_temp_f"
        case wateringMethod = "watering_method"
        case accessibility
        case haArea = "ha_area"
        case potSizeIn = "pot_size_in"
        case potMaterial = "pot_material"
        case hasDrainage = "has_drainage"
        case toxicity
    }
}

struct PlantFromPhoto: Sendable {
    var jpeg: Data
    var metadata: CaptureMetadata
    var commonName: String?
    var location: String?
    var steward: String?
}

struct PlantFromPhotoResult: Sendable {
    let plant: Plant
    let candidates: [IdentificationCandidate]
    let photoError: String?
}

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

extension PlantyClient {
    func homeAssistantEntities() async throws -> [HomeAssistantEntity] {
        let response: HomeAssistantEntityListResponse = try await get(APIPath.listHomeAssistantEntities)
        return response.entities
    }
}
