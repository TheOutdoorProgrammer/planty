import Foundation

extension PlantyClient {
    func managedChoices() async throws -> ManagedChoices {
        try await get(APIPath.listChoices)
    }
}
