import Foundation

extension PlantyClient {
    func plantHealth(slug: String) async throws -> PlantHealthResponse {
        try await get(APIPath.getPlantHealth(slug: escaped(slug)))
    }

    func addHealthEvent(slug: String, change: NewHealthChange) async throws -> HealthEvent {
        try await send(
            "POST",
            APIPath.addHealthEvent(slug: escaped(slug)),
            body: change
        )
    }
}
