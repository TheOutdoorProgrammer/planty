import Foundation

/// URLSession implementation of the contract. Holds no state beyond the
/// configuration it was built with, so a settings change makes a new one.
struct PlantyClient: PlantyAPI {
    let configuration: PlantyConfiguration
    let session: URLSession
    let images: ImageRepository?

    /// The session used for anything waiting on a model. Injected alongside the
    /// ordinary one so a test can drive both through the same stub.
    let patientSession: URLSession

    init(
        configuration: PlantyConfiguration,
        session: URLSession = .plantyDefault,
        patientSession: URLSession? = nil,
        images: ImageRepository? = nil
    ) {
        self.configuration = configuration
        self.session = session
        self.images = images
        self.patientSession = patientSession ?? (session === URLSession.plantyDefault
            ? .plantyPatient
            : session)
    }

    func today() async throws -> Digest {
        try await get(APIPath.today)
    }

    func plants(filter: PlantFilter) async throws -> [Plant] {
        let response: PlantListResponse = try await get(APIPath.listPlants, query: filter.queryItems)
        return response.plants
    }

    func plant(slug: String) async throws -> PlantDetail {
        try await get(APIPath.getPlant(slug: escaped(slug)))
    }

    func createPlant(_ draft: NewPlant) async throws -> Plant {
        try await send("POST", APIPath.createPlant, body: draft)
    }

    func updatePlant(slug: String, patch: PlantPatch) async throws -> Plant {
        try await send("PATCH", APIPath.updatePlant(slug: escaped(slug)), body: patch)
    }

    func archivePlant(slug: String, status: PlantStatus) async throws {
        let request = try makeRequest(
            "DELETE",
            APIPath.archivePlant(slug: escaped(slug)),
            query: [URLQueryItem(name: "status", value: status.rawValue)]
        )
        _ = try await perform(request)
    }

    func restorePlant(slug: String) async throws -> Plant {
        try await send("POST", APIPath.restorePlant(slug: escaped(slug)), body: EmptyBody())
    }

    func addObservation(slug: String, observation: NewObservation) async throws -> PlantObservation {
        try await send("POST", APIPath.addObservation(slug: escaped(slug)), body: observation)
    }

    func timeline(slug: String) async throws -> PlantTimeline {
        try await get(APIPath.getTimeline(slug: escaped(slug)))
    }

    func ask(slug: String, question: PlantQuestion) async throws -> PlantAnswer {
        try await send("POST", APIPath.askPlant(slug: escaped(slug)),
                       body: question, patience: Patience.model)
    }

    func assess(slug: String) async throws -> Verdict {
        try await send("POST", APIPath.assessPlant(slug: escaped(slug)),
                       body: EmptyBody(), patience: Patience.model)
    }

    func conversations(slug: String) async throws -> [PlantConversationSummary] {
        let response: PlantConversationListResponse = try await get(
            APIPath.listPlantConversations(slug: escaped(slug))
        )
        return response.conversations
    }

    func conversation(slug: String, id: UUID) async throws -> PlantConversation {
        try await get(APIPath.getPlantConversation(
            slug: escaped(slug), id: id.uuidString
        ))
    }

    func ask(_ question: ScratchQuestion) async throws -> PlantAnswer {
        try await send("POST", APIPath.ask, body: question, patience: Patience.model)
    }

    /// Naming, creating and keeping the photograph are one call, so a first
    /// plant cannot end up half-made with its picture thrown away.
    func createPlantFromPhoto(_ ask: PlantFromPhoto) async throws -> PlantFromPhotoResult {
        var query: [URLQueryItem] = []
        if let capturedAt = ask.metadata.capturedAt {
            query.append(URLQueryItem(name: "taken_at", value: PlantyDateFormat.string(from: capturedAt)))
        }
        if let latitude = ask.metadata.latitude, let longitude = ask.metadata.longitude {
            query.append(URLQueryItem(name: "lat", value: String(latitude)))
            query.append(URLQueryItem(name: "lon", value: String(longitude)))
        }
        for (name, value) in [
            ("common_name", ask.commonName), ("location", ask.location), ("steward", ask.steward)
        ] {
            if let value, !value.isEmpty { query.append(URLQueryItem(name: name, value: value)) }
        }

        var body = MultipartBody()
        body.appendFile(name: "photo", filename: "subject.jpg", contentType: "image/jpeg", data: ask.jpeg)

        var request = try makeRequest("POST", APIPath.createPlantFromPhoto, query: query,
                                      patience: Patience.model)
        request.setValue(body.contentType, forHTTPHeaderField: "Content-Type")
        request.httpBody = body.finished()

        let response: PlantFromPhotoResponse = try decode(
            PlantFromPhotoResponse.self, from: try await perform(request, patient: true)
        )
        return PlantFromPhotoResult(
            plant: response.plant,
            candidates: response.candidates ?? [],
            photoError: response.photoError
        )
    }

    func linkSensor(_ link: NewSensorLink) async throws -> SensorLink {
        try await send("POST", APIPath.linkSensor, body: link)
    }

    func postmortem(slug: String) async throws -> Postmortem {
        try await send("POST", APIPath.createPostmortem(slug: escaped(slug)),
                       body: EmptyBody(), patience: Patience.model)
    }

    func postmortems() async throws -> [Postmortem] {
        let response: PostmortemListResponse = try await get(APIPath.listPostmortems)
        return response.postmortems
    }

    func questions(status: QuestionStatus) async throws -> [OpenQuestion] {
        let response: QuestionListResponse = try await get(
            APIPath.listQuestions,
            query: [URLQueryItem(name: "status", value: status.rawValue)]
        )
        return response.questions
    }

    func createQuestion(_ draft: NewOpenQuestion) async throws -> OpenQuestion {
        try await send("POST", APIPath.createQuestion, body: draft)
    }

    func planAway(_ draft: NewAwayPeriod) async throws -> AwayPeriod {
        try await send("POST", APIPath.createAway, body: draft)
    }

    func coldWatch(forecastLowF: Double) async throws -> ColdWatch {
        try await get(
            APIPath.coldWatch,
            query: [URLQueryItem(name: "forecast_low_f", value: String(forecastLowF))]
        )
    }

    func notes(slug: String) async throws -> [PlantNote] {
        let response: NoteListResponse = try await get(APIPath.listPlantNotes(slug: escaped(slug)))
        return response.notes
    }

    func answerQuestion(id: UUID, answer: String) async throws {
        var request = try makeRequest("POST", APIPath.answerQuestion(id: id.uuidString))
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try PlantyCoders.encoder().encode(QuestionAnswer(answer: answer))
        _ = try await perform(request)
    }

    func householdNotes() async throws -> [PlantNote] {
        let response: NoteListResponse = try await get(APIPath.listHouseholdNotes)
        return response.notes
    }

    func addHouseholdNote(draft: NoteDraft) async throws -> PlantNote {
        try await send("POST", APIPath.addHouseholdNote, body: draft)
    }

    func addNote(slug: String, draft: NoteDraft) async throws -> PlantNote {
        try await send("POST", APIPath.addPlantNote(slug: escaped(slug)), body: draft)
    }

    func updateNote(id: UUID, draft: NoteDraft) async throws -> PlantNote {
        try await send("PATCH", APIPath.updateNote(id: id.uuidString), body: draft)
    }

    func deleteNote(id: UUID) async throws {
        _ = try await perform(try makeRequest("DELETE", APIPath.deleteNote(id: id.uuidString)))
    }

    func reminders(slug: String) async throws -> [Reminder] {
        let response: ReminderListResponse = try await get(APIPath.listReminders(slug: escaped(slug)))
        return response.reminders
    }

    func setReminder(slug: String, reminder: NewReminder) async throws -> Reminder {
        try await send("PUT", APIPath.setReminder(slug: escaped(slug)), body: reminder)
    }

    func deleteReminder(slug: String, kind: ObservationKind) async throws {
        let path = APIPath.deleteReminder(slug: escaped(slug), kind: escaped(kind.rawValue))
        _ = try await perform(try makeRequest("DELETE", path))
    }

    func acknowledge(verdictID: UUID) async throws {
        let request = try makeRequest("POST", APIPath.ackVerdict(id: verdictID.uuidString))
        _ = try await perform(request)
    }

    func sensors() async throws -> [SensorLink] {
        let response: SensorListResponse = try await get(APIPath.listSensors)
        return response.sensors
    }

    func calibrate(sensorID: UUID, to calibration: SensorCalibration) async throws -> SensorLink {
        try await send("PATCH", APIPath.calibrateSensor(id: sensorID.uuidString), body: calibration)
    }

    func shelter(slugs: [String], indoors: Bool) async throws -> Int {
        let moved: ShelterResponse = try await send(
            "POST",
            indoors ? APIPath.shelter : APIPath.unshelter,
            body: ShelterRequest(slugs: slugs)
        )
        return moved.moved
    }

    func logHarvest(_ harvest: NewHarvest, on slug: String) async throws -> Harvest {
        try await send("POST", APIPath.addHarvest(slug: escaped(slug)), body: harvest)
    }

    func harvests(slug: String? = nil) async throws -> [Harvest] {
        let path = slug.map { APIPath.listPlantHarvests(slug: escaped($0)) } ?? APIPath.listHarvests
        let response: HarvestListResponse = try await get(path)
        return response.harvests
    }

    func harvestSummary() async throws -> [HarvestSummary] {
        let response: HarvestSummaryResponse = try await get(APIPath.harvestSummary)
        return response.summary
    }

    func updateHarvest(id: UUID, draft: NewHarvest) async throws -> Harvest {
        try await send("PATCH", APIPath.updateHarvest(id: id.uuidString), body: draft)
    }

    func deleteHarvest(id: UUID) async throws {
        _ = try await perform(try makeRequest("DELETE", APIPath.deleteHarvest(id: id.uuidString)))
    }

    func deletePhoto(id: UUID) async throws {
        _ = try await perform(try makeRequest("DELETE", APIPath.deletePhoto(id: id.uuidString)))
    }

    func health() async throws {
        _ = try await perform(try makeRequest("GET", APIPath.health))
    }

    /// Species identification. The capture date and coordinate ride along as
    /// query items because region and season narrow the candidate set.
    func identify(jpeg: Data, metadata: CaptureMetadata) async throws -> [IdentificationCandidate] {
        var query: [URLQueryItem] = []
        if let capturedAt = metadata.capturedAt {
            query.append(URLQueryItem(name: "taken_at", value: PlantyDateFormat.string(from: capturedAt)))
        }
        if let latitude = metadata.latitude, let longitude = metadata.longitude {
            query.append(URLQueryItem(name: "lat", value: String(latitude)))
            query.append(URLQueryItem(name: "lon", value: String(longitude)))
        }

        var body = MultipartBody()
        body.appendFile(name: "photo", filename: "subject.jpg", contentType: "image/jpeg", data: jpeg)

        var request = try makeRequest("POST", APIPath.identify, query: query,
                                      patience: Patience.model)
        request.setValue(body.contentType, forHTTPHeaderField: "Content-Type")
        request.httpBody = body.finished()

        let response: IdentifyResponse = try decode(
            IdentifyResponse.self, from: try await perform(request, patient: true)
        )
        return response.candidates
    }

    func uploadPhoto(slug: String, jpeg: Data, caption: String?, takenAt: Date) async throws -> Photo {
        var body = MultipartBody()
        body.appendFile(
            name: "photo",
            filename: "\(slug)-\(Int(takenAt.timeIntervalSince1970)).jpg",
            contentType: "image/jpeg",
            data: jpeg
        )
        body.appendField(name: "taken_at", value: PlantyDateFormat.string(from: takenAt))
        if let caption, !caption.isEmpty {
            body.appendField(name: "caption", value: caption)
        }

        var request = try makeRequest("POST", APIPath.uploadPhoto(slug: escaped(slug)))
        request.setValue(body.contentType, forHTTPHeaderField: "Content-Type")
        request.httpBody = body.finished()
        let photo = try decode(Photo.self, from: try await perform(request))
        if let images {
            await images.seed(jpeg, for: .photo(photo, rendition: .original))
            await images.seed(jpeg, for: .photo(photo, rendition: .thumbnail))
            await images.seed(
                jpeg,
                for: .cover(slug: slug, changedAt: photo.takenAt, rendition: .original)
            )
            await images.seed(
                jpeg,
                for: .cover(slug: slug, changedAt: photo.takenAt, rendition: .thumbnail)
            )
        }
        return photo
    }
}

/// Shared by the small endpoint-specific extensions too. There is one request,
/// status and decoding boundary for the whole client; only route/body shapes
/// vary by operation.
extension PlantyClient {
    func get<T: Decodable>(_ path: String, query: [URLQueryItem] = []) async throws -> T {
        try decode(T.self, from: try await perform(try makeRequest("GET", path, query: query)))
    }

    func send<Body: Encodable, T: Decodable>(
        _ method: String,
        _ path: String,
        body: Body,
        patience: TimeInterval = Patience.ordinary
    ) async throws -> T {
        var request = try makeRequest(method, path, patience: patience)
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try PlantyCoders.encoder().encode(body)
        return try decode(T.self, from: try await perform(request, patient: patience > Patience.ordinary))
    }

    func makeRequest(
        _ method: String,
        _ path: String,
        query: [URLQueryItem] = [],
        patience: TimeInterval = Patience.ordinary
    ) throws -> URLRequest {
        guard let baseURL = configuration.baseURL else { throw PlantyError.notConfigured }
        guard var components = URLComponents(
            url: baseURL.appendingPathComponent(path),
            resolvingAgainstBaseURL: false
        ) else {
            throw PlantyError.transport("Could not build a URL for \(path).")
        }
        if !query.isEmpty { components.queryItems = query }
        guard let url = components.url else {
            throw PlantyError.transport("Could not build a URL for \(path).")
        }

        var request = URLRequest(url: url, timeoutInterval: patience)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let token = configuration.token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        return request
    }

    func perform(_ request: URLRequest, patient: Bool = false) async throws -> Data {
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await (patient ? patientSession : session).data(for: request)
        } catch {
            throw PlantyError.from(error)
        }

        guard let http = response as? HTTPURLResponse else {
            throw PlantyError.transport("The service answered with no status.")
        }
        guard (200..<300).contains(http.statusCode) else {
            throw statusError(http.statusCode, data)
        }
        return data
    }

    func statusError(_ status: Int, _ data: Data) -> PlantyError {
        let message = try? JSONDecoder().decode(APIErrorBody.self, from: data).error
        switch status {
        case 401, 403: return .unauthorized
        case 404: return .notFound
        default: return .server(status: status, message: message)
        }
    }

    func decode<T: Decodable>(_ type: T.Type, from data: Data) throws -> T {
        do {
            return try PlantyCoders.decoder().decode(type, from: data)
        } catch {
            throw PlantyError.from(error)
        }
    }

    /// Slugs are human-written, so a stray space must not build a broken path.
    func escaped(_ slug: String) -> String {
        slug.addingPercentEncoding(withAllowedCharacters: .urlPathAllowed) ?? slug
    }
}

extension URLSession {
    /// Short timeouts on purpose: a hung request must become a visible error,
    /// never a screen that keeps implying it is about to reassure you.
    static let plantyDefault: URLSession = {
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = Patience.ordinary
        config.timeoutIntervalForResource = 60
        config.waitsForConnectivity = false
        return URLSession(configuration: config)
    }()

    /// For calls that wait on a model. Its own session because
    /// `timeoutIntervalForResource` belongs to the session and caps the whole
    /// load, so raising a request's timeout past it achieves nothing.
    static let plantyPatient: URLSession = {
        let config = URLSessionConfiguration.default
        config.timeoutIntervalForRequest = Patience.model
        config.timeoutIntervalForResource = Patience.model + 60
        config.waitsForConnectivity = false
        return URLSession(configuration: config)
    }()
}

/// How long a request is allowed to take. Reading a record is either quick or
/// broken; waiting on a model is neither, and holding both to the same limit
/// turned a working answer into "the service did not answer in time".
enum Patience {
    /// Fifteen seconds. Anything reading the database.
    static let ordinary: TimeInterval = 15

    /// Three minutes. Anything that waits on a model: a judgment can open a
    /// photograph or run a command, and measured replies reach twenty seconds
    /// before either of those.
    static let model: TimeInterval = 180
}
