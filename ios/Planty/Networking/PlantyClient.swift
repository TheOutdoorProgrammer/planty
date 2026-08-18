import Foundation

/// URLSession implementation of the contract. Holds no state beyond the
/// configuration it was built with, so a settings change makes a new one.
struct PlantyClient: PlantyAPI {
    let configuration: PlantyConfiguration
    let session: URLSession

    init(configuration: PlantyConfiguration, session: URLSession = .plantyDefault) {
        self.configuration = configuration
        self.session = session
    }

    func today() async throws -> Digest {
        try await get("/v1/today")
    }

    func plants(filter: PlantFilter) async throws -> [Plant] {
        let response: PlantListResponse = try await get("/v1/plants", query: filter.queryItems)
        return response.plants
    }

    func plant(slug: String) async throws -> PlantDetail {
        try await get("/v1/plants/\(escaped(slug))")
    }

    func createPlant(_ draft: NewPlant) async throws -> Plant {
        try await send("POST", "/v1/plants", body: draft)
    }

    func updatePlant(slug: String, patch: PlantPatch) async throws -> Plant {
        try await send("PATCH", "/v1/plants/\(escaped(slug))", body: patch)
    }

    func archivePlant(slug: String, status: PlantStatus) async throws {
        let request = try makeRequest(
            "DELETE",
            "/v1/plants/\(escaped(slug))",
            query: [URLQueryItem(name: "status", value: status.rawValue)]
        )
        _ = try await perform(request)
    }

    func addObservation(slug: String, observation: NewObservation) async throws -> PlantObservation {
        try await send("POST", "/v1/plants/\(escaped(slug))/observations", body: observation)
    }

    func timeline(slug: String) async throws -> PlantTimeline {
        try await get("/v1/plants/\(escaped(slug))/timeline")
    }

    func acknowledge(verdictID: UUID) async throws {
        let request = try makeRequest("POST", "/v1/verdicts/\(verdictID.uuidString)/ack")
        _ = try await perform(request)
    }

    func sensors() async throws -> [SensorLink] {
        let response: SensorListResponse = try await get("/v1/sensors")
        return response.sensors
    }

    func calibrate(sensorID: UUID, to calibration: SensorCalibration) async throws -> SensorLink {
        try await send("PATCH", "/v1/sensors/\(sensorID.uuidString)", body: calibration)
    }

    func logHarvest(_ harvest: NewHarvest, on slug: String) async throws -> Harvest {
        try await send("POST", "/v1/plants/\(escaped(slug))/harvests", body: harvest)
    }

    func health() async throws {
        _ = try await perform(try makeRequest("GET", "/healthz"))
    }

    /// The multipart field names are an assumption: DATA-MODEL.md documents the
    /// endpoint but not its body. See ios/README.md.
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

        var request = try makeRequest("POST", "/v1/plants/\(escaped(slug))/photos")
        request.setValue(body.contentType, forHTTPHeaderField: "Content-Type")
        request.httpBody = body.finished()
        return try decode(Photo.self, from: try await perform(request))
    }
}

private extension PlantyClient {
    func get<T: Decodable>(_ path: String, query: [URLQueryItem] = []) async throws -> T {
        try decode(T.self, from: try await perform(try makeRequest("GET", path, query: query)))
    }

    func send<Body: Encodable, T: Decodable>(
        _ method: String,
        _ path: String,
        body: Body
    ) async throws -> T {
        var request = try makeRequest(method, path)
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try PlantyCoders.encoder().encode(body)
        return try decode(T.self, from: try await perform(request))
    }

    func makeRequest(
        _ method: String,
        _ path: String,
        query: [URLQueryItem] = []
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

        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let token = configuration.token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        return request
    }

    func perform(_ request: URLRequest) async throws -> Data {
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
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
        config.timeoutIntervalForRequest = 15
        config.timeoutIntervalForResource = 60
        config.waitsForConnectivity = false
        return URLSession(configuration: config)
    }()
}
