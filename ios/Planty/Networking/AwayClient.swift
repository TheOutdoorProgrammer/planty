import Foundation

extension PlantyClient {
    func awayPeriods() async throws -> [AwayPeriod] {
        let response: AwayPeriodListResponse = try await get(APIPath.listAway)
        return response.awayPeriods
    }

    func updateAway(id: UUID, draft: NewAwayPeriod) async throws -> AwayPeriod {
        try await send("PATCH", APIPath.updateAway(id: id.uuidString), body: AwayPeriodUpdate(draft))
    }

    func cancelAway(id: UUID) async throws {
        _ = try await perform(try makeRequest("DELETE", APIPath.deleteAway(id: id.uuidString)))
    }
}
