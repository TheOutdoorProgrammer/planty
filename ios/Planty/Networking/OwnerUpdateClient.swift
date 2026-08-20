import Foundation

extension PlantyClient {
    func registerPushDevice(_ device: PushDeviceRegistration) async throws {
        var request = try makeRequest("POST", APIPath.registerPushDevice)
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try PlantyCoders.encoder().encode(device)
        _ = try await perform(request)
    }

    func ownerUpdate(steward: String) async throws -> OwnerUpdate {
        try await send(
            "POST",
            APIPath.createOwnerUpdate,
            body: OwnerUpdateRequest(steward: steward),
            patience: Patience.model
        )
    }
}
