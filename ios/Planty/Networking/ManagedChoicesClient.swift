import Foundation

extension PlantyClient {
    func managedChoices() async throws -> ManagedChoices {
        guard let baseURL = configuration.baseURL else { throw PlantyError.notConfigured }
        let url = baseURL.appendingPathComponent("/v1/choices")
        var request = URLRequest(url: url, timeoutInterval: Patience.ordinary)
        request.httpMethod = "GET"
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if let token = configuration.token {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }

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
            switch http.statusCode {
            case 401, 403: throw PlantyError.unauthorized
            case 404: throw PlantyError.notFound
            default: throw PlantyError.server(status: http.statusCode, message: nil)
            }
        }
        do {
            return try PlantyCoders.decoder().decode(ManagedChoices.self, from: data)
        } catch {
            throw PlantyError.from(error)
        }
    }
}
