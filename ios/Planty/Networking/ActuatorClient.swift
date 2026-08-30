import Foundation

extension PlantyClient {
    func discoverActuators() async throws -> [HomeAssistantEntity] {
        let response: DiscoveredActuatorListResponse = try await get(APIPath.discoverActuators)
        return response.entities
    }

    func actuators() async throws -> [Actuator] {
        let response: ActuatorListResponse = try await get(APIPath.listActuators)
        return response.actuators
    }

    func registerActuator(_ registration: ActuatorRegistration) async throws -> Actuator {
        try await send("POST", APIPath.registerActuator, body: registration)
    }

    func renameActuator(
        id: UUID,
        name: String,
        plantIDs: [UUID],
        policyControlEnabled: Bool
    ) async throws -> Actuator {
        try await send(
            "PATCH",
            APIPath.updateActuator(id: id.uuidString),
            body: ActuatorRename(
                name: name,
                plantIDs: plantIDs,
                policyControlEnabled: policyControlEnabled
            )
        )
    }

    func deleteActuator(id: UUID) async throws {
        _ = try await perform(try makeRequest("DELETE", APIPath.deleteActuator(id: id.uuidString)))
    }

    func actuatorEvents(id: UUID) async throws -> [ActuatorEvent] {
        let response: ActuatorEventListResponse = try await get(
            APIPath.listActuatorEvents(id: id.uuidString)
        )
        return response.events
    }

    func startActuator(id: UUID, request: ActuatorStartRequest) async throws -> ActuatorLease {
        try await send("POST", APIPath.startActuator(id: id.uuidString), body: request)
    }

    func stopActuator(id: UUID, request: ActuatorStopRequest) async throws -> ActuatorStopResponse {
        try await send("POST", APIPath.stopActuator(id: id.uuidString), body: request)
    }

    func setLightState(id: UUID, request: LightStateRequest) async throws {
        let _: LightStateResponse = try await send(
            "POST",
            APIPath.setActuatorState(id: id.uuidString),
            body: request
        )
    }

    func setLightSchedule(id: UUID, request: LightScheduleRequest) async throws -> LightSchedule {
        try await send("PUT", APIPath.setLightSchedule(id: id.uuidString), body: request)
    }

    func deleteLightSchedule(id: UUID) async throws {
        _ = try await perform(try makeRequest(
            "DELETE",
            APIPath.deleteLightSchedule(id: id.uuidString)
        ))
    }
}
