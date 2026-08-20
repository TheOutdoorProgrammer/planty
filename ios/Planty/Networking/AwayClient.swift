import Foundation

extension PlantyClient {
    func awayPeriods() async throws -> [AwayPeriod] {
        let request = try awayRequest("GET", "/v1/away")
        let data = try await awayPerform(request)
        do {
            return try PlantyCoders.decoder().decode(AwayPeriodListResponse.self, from: data).awayPeriods
        } catch {
            throw PlantyError.from(error)
        }
    }

    func updateAway(id: UUID, draft: NewAwayPeriod) async throws -> AwayPeriod {
        var request = try awayRequest("PATCH", "/v1/away/\(id.uuidString)")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try PlantyCoders.encoder().encode(AwayPeriodUpdate(draft))
        let data = try await awayPerform(request)
        do {
            return try PlantyCoders.decoder().decode(AwayPeriod.self, from: data)
        } catch {
            throw PlantyError.from(error)
        }
    }

    func cancelAway(id: UUID) async throws {
        _ = try await awayPerform(try awayRequest("DELETE", "/v1/away/\(id.uuidString)"))
    }
}

private struct AwayAPIError: Decodable {
    let error: String
}

private extension PlantyClient {
    func awayRequest(_ method: String, _ path: String) throws -> URLRequest {
        guard let baseURL = configuration.baseURL else { throw PlantyError.notConfigured }
        let url = baseURL.appendingPathComponent(path)
        var request = URLRequest(url: url, timeoutInterval: Patience.ordinary)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let token = configuration.token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        return request
    }

    func awayPerform(_ request: URLRequest) async throws -> Data {
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
            let message = try? JSONDecoder().decode(AwayAPIError.self, from: data).error
            switch http.statusCode {
            case 401, 403: throw PlantyError.unauthorized
            case 404: throw PlantyError.notFound
            default: throw PlantyError.server(status: http.statusCode, message: message)
            }
        }
        return data
    }
}
