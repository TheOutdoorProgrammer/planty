import Foundation

extension PlantyClient {
    func registerPushDevice(_ device: PushDeviceRegistration) async throws -> PushRegistrationReceipt {
        try await send("POST", APIPath.registerPushDevice, body: device)
    }

    func pushHealth(installationID: UUID, environment: String) async throws -> PushHealth {
        try await get(APIPath.pushHealth, query: [
            URLQueryItem(name: "installation_id", value: installationID.uuidString),
            URLQueryItem(name: "environment", value: environment),
        ])
    }

    func testPush(_ body: PushInstallationRequest) async throws {
        var request = try makeRequest("POST", APIPath.testPush)
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try PlantyCoders.encoder().encode(body)
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
