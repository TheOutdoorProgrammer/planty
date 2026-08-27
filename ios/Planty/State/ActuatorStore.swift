import Foundation
import Observation

@Observable
@MainActor
final class ActuatorStore {
    private(set) var registered: [Actuator] = []
    private(set) var discovered: [HomeAssistantEntity] = []
    private(set) var events: [UUID: [ActuatorEvent]] = [:]
    private(set) var leases: [UUID: ActuatorLease] = [:]
    private(set) var error: PlantyError?
    private(set) var hasLoaded = false
    private(set) var isDiscovering = false
    private(set) var controlling: Set<UUID> = []

    private var api: any PlantyAPI
    private var isConfigured: Bool
    private var generation = 0
    private var startKeys: [UUID: UUID] = [:]
    private var stopKeys: [UUID: UUID] = [:]

    init(api: any PlantyAPI, isConfigured: Bool) {
        self.api = api
        self.isConfigured = isConfigured
    }

    func replace(api: any PlantyAPI, isConfigured: Bool) {
        generation += 1
        self.api = api
        self.isConfigured = isConfigured
        registered = []
        discovered = []
        events = [:]
        leases = [:]
        error = nil
        hasLoaded = false
        isDiscovering = false
        controlling = []
        startKeys = [:]
        stopKeys = [:]
    }

    func load() async {
        guard isConfigured else {
            registered = []
            error = nil
            hasLoaded = true
            return
        }
        let startedGeneration = generation
        do {
            let loaded = try await api.actuators()
            guard startedGeneration == generation else { return }
            registered = loaded
            leases = [:]
            for actuator in loaded {
                if let lease = actuator.activeLease, lease.isActive { leases[actuator.id] = lease }
                await loadEvents(for: actuator)
            }
            error = nil
            hasLoaded = true
        } catch {
            guard startedGeneration == generation, !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
            hasLoaded = true
        }
    }

    func discover() async {
        guard !isDiscovering else { return }
        let startedGeneration = generation
        let client = api
        isDiscovering = true
        defer { if startedGeneration == generation { isDiscovering = false } }
        do {
            let loaded = try await client.discoverActuators()
                .filter { $0.domain == "fan" || $0.domain == "switch" }
                .sorted {
                    if $0.available != $1.available { return $0.available }
                    return $0.friendlyName.localizedCaseInsensitiveCompare($1.friendlyName) == .orderedAscending
                }
            guard startedGeneration == generation else { return }
            discovered = loaded
            error = nil
        } catch {
            guard startedGeneration == generation, !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
    }

    func register(entity: HomeAssistantEntity, name: String, plantIDs: [UUID]) async -> PlantyError? {
        guard discovered.contains(where: { $0.entityID == entity.entityID }),
              entity.domain == "fan" || entity.domain == "switch"
        else { return .transport("Choose a discovered Home Assistant fan or switch.") }
        let cleanedName = name.cleaned
        guard !cleanedName.isEmpty else { return .transport("Give the actuator a name.") }
        guard !plantIDs.isEmpty else { return .transport("Choose at least one plant this actuator serves.") }
        do {
            let saved = try await api.registerActuator(
                ActuatorRegistration(entityID: entity.entityID, name: cleanedName, plantIDs: plantIDs)
            )
            registered.removeAll { $0.id == saved.id }
            registered.append(saved)
            registered.sort { $0.name.localizedCaseInsensitiveCompare($1.name) == .orderedAscending }
            error = nil
            return nil
        } catch {
            return record(error)
        }
    }

    func remove(_ actuator: Actuator) async -> PlantyError? {
        do {
            try await api.deleteActuator(id: actuator.id)
            registered.removeAll { $0.id == actuator.id }
            events.removeValue(forKey: actuator.id)
            leases.removeValue(forKey: actuator.id)
            error = nil
            return nil
        } catch {
            return record(error)
        }
    }

    func update(
        _ actuator: Actuator,
        name: String,
        plantIDs: [UUID],
        policyControlEnabled: Bool
    ) async -> PlantyError? {
        let cleanedName = name.cleaned
        guard !cleanedName.isEmpty else { return .transport("Give the actuator a name.") }
        guard !plantIDs.isEmpty else { return .transport("Choose at least one plant this actuator serves.") }
        do {
            let saved = try await api.renameActuator(
                id: actuator.id,
                name: cleanedName,
                plantIDs: plantIDs,
                policyControlEnabled: policyControlEnabled
            )
            if let index = registered.firstIndex(where: { $0.id == saved.id }) {
                registered[index] = saved
            }
            error = nil
            return nil
        } catch {
            return record(error)
        }
    }

    func start(_ actuator: Actuator, durationSeconds: Int) async -> PlantyError? {
        guard (1...3_600).contains(durationSeconds) else {
            return .transport("Run time must be between one second and one hour.")
        }
        guard !controlling.contains(actuator.id) else { return nil }
        controlling.insert(actuator.id)
        defer { controlling.remove(actuator.id) }
        let key = startKeys[actuator.id] ?? UUID()
        startKeys[actuator.id] = key
        do {
            let lease = try await api.startActuator(
                id: actuator.id,
                request: ActuatorStartRequest(
                    durationSeconds: durationSeconds,
                    actor: "owner",
                    idempotencyKey: key
                )
            )
            leases[actuator.id] = lease
            startKeys[actuator.id] = UUID()
            error = nil
            await loadEvents(for: actuator)
            return nil
        } catch {
            return record(error)
        }
    }

    func stop(_ actuator: Actuator) async -> PlantyError? {
        guard !controlling.contains(actuator.id) else { return nil }
        controlling.insert(actuator.id)
        defer { controlling.remove(actuator.id) }
        let key = stopKeys[actuator.id] ?? UUID()
        stopKeys[actuator.id] = key
        do {
            _ = try await api.stopActuator(
                id: actuator.id,
                request: ActuatorStopRequest(actor: "owner", idempotencyKey: key)
            )
            leases.removeValue(forKey: actuator.id)
            stopKeys[actuator.id] = UUID()
            error = nil
            await loadEvents(for: actuator)
            return nil
        } catch {
            return record(error)
        }
    }

    func loadEvents(for actuator: Actuator) async {
        let startedGeneration = generation
        let client = api
        do {
            let loaded = try await client.actuatorEvents(id: actuator.id).sorted {
                $0.createdAt > $1.createdAt
            }
            guard startedGeneration == generation else { return }
            events[actuator.id] = loaded
        } catch {
            guard startedGeneration == generation, !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
    }

    private func record(_ underlying: Error) -> PlantyError? {
        guard !PlantyError.isCancellation(underlying) else { return nil }
        let failure = PlantyError.from(underlying)
        error = failure
        return failure
    }
}
