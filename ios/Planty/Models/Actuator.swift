import Foundation

struct Actuator: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let entityID: String
    var name: String
    let kind: ActuatorKind
    var plantIDs: [UUID]
    var policyControlEnabled: Bool = false
    var currentState: String?
    let createdAt: Date
    let updatedAt: Date
    var activeLease: ActuatorLease?
    var lightSchedule: LightSchedule?
    var fanSchedule: ActuatorSchedule?

    enum CodingKeys: String, CodingKey {
        case id
        case entityID = "entity_id"
        case name
        case kind
        case plantIDs = "plant_ids"
        case policyControlEnabled = "policy_control_enabled"
        case currentState = "current_state"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
        case activeLease = "active_lease"
        case lightSchedule = "light_schedule"
        case fanSchedule = "fan_schedule"
    }
}

extension Actuator {
    init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(UUID.self, forKey: .id)
        entityID = try container.decode(String.self, forKey: .entityID)
        name = try container.decode(String.self, forKey: .name)
        kind = try container.decode(ActuatorKind.self, forKey: .kind)
        plantIDs = try container.decode([UUID].self, forKey: .plantIDs)
        policyControlEnabled = try container.decodeIfPresent(Bool.self, forKey: .policyControlEnabled) ?? false
        currentState = try container.decodeIfPresent(String.self, forKey: .currentState)
        createdAt = try container.decode(Date.self, forKey: .createdAt)
        updatedAt = try container.decode(Date.self, forKey: .updatedAt)
        activeLease = try container.decodeIfPresent(ActuatorLease.self, forKey: .activeLease)
        lightSchedule = try container.decodeIfPresent(LightSchedule.self, forKey: .lightSchedule)
        fanSchedule = try container.decodeIfPresent(ActuatorSchedule.self, forKey: .fanSchedule)
    }
}

extension Actuator {
    var dailySchedule: ActuatorSchedule? {
        switch kind {
        case .fan: fanSchedule
        case .light: lightSchedule
        default: nil
        }
    }

    var isOn: Bool? {
        switch currentState?.lowercased() {
        case "on": true
        case "off": false
        default: nil
        }
    }

    var stateLabel: String {
        switch isOn {
        case true: "On"
        case false: "Off"
        case nil where currentState == "unavailable": "Unavailable"
        default: "Status unknown"
        }
    }
}

struct ActuatorSchedule: Codable, Sendable, Hashable {
    let actuatorID: UUID
    let startMinute: Int
    let endMinute: Int
    let windows: [ActuatorScheduleWindow]?
    let timezone: String
    let enabled: Bool
    let lastAppliedState: Bool?
    let lastAppliedAt: Date?
    let lastError: String?
    let createdAt: Date
    let updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case actuatorID = "actuator_id"
        case startMinute = "start_minute"
        case endMinute = "end_minute"
        case windows
        case timezone
        case enabled
        case lastAppliedState = "last_applied_state"
        case lastAppliedAt = "last_applied_at"
        case lastError = "last_error"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }

    init(
        actuatorID: UUID,
        startMinute: Int,
        endMinute: Int,
        timezone: String,
        enabled: Bool,
        lastAppliedState: Bool?,
        lastAppliedAt: Date?,
        lastError: String?,
        createdAt: Date,
        updatedAt: Date,
        windows: [ActuatorScheduleWindow]? = nil
    ) {
        self.actuatorID = actuatorID
        self.startMinute = startMinute
        self.endMinute = endMinute
        self.windows = windows
        self.timezone = timezone
        self.enabled = enabled
        self.lastAppliedState = lastAppliedState
        self.lastAppliedAt = lastAppliedAt
        self.lastError = lastError
        self.createdAt = createdAt
        self.updatedAt = updatedAt
    }

    var dailyWindows: [ActuatorScheduleWindow] {
        if let windows, !windows.isEmpty { return windows }
        return [ActuatorScheduleWindow(startMinute: startMinute, endMinute: endMinute)]
    }
}

struct ActuatorScheduleWindow: Codable, Sendable, Hashable {
    let startMinute: Int
    let endMinute: Int

    enum CodingKeys: String, CodingKey {
        case startMinute = "start_minute"
        case endMinute = "end_minute"
    }
}

typealias LightSchedule = ActuatorSchedule

struct LightScheduleRequest: Codable, Sendable, Equatable {
    let startMinute: Int
    let endMinute: Int
    let windows: [ActuatorScheduleWindow]
    let timezone: String
    let enabled: Bool
    let actor: String

    enum CodingKeys: String, CodingKey {
        case startMinute = "start_minute"
        case endMinute = "end_minute"
        case windows
        case timezone, enabled, actor
    }

    init(windows: [ActuatorScheduleWindow], timezone: String, enabled: Bool, actor: String) {
        startMinute = windows.first?.startMinute ?? 0
        endMinute = windows.first?.endMinute ?? 0
        self.windows = windows
        self.timezone = timezone
        self.enabled = enabled
        self.actor = actor
    }
}

struct LightScheduleDraft: Equatable {
    var windows: [ActuatorScheduleWindowDraft]
    var timezone: String
    var enabled: Bool

    init(schedule: ActuatorSchedule?, defaultTimezone: String = TimeZone.current.identifier) {
        windows = (schedule?.dailyWindows ?? [ActuatorScheduleWindow(startMinute: 7 * 60, endMinute: 21 * 60)])
            .map(ActuatorScheduleWindowDraft.init)
        timezone = schedule?.timezone ?? defaultTimezone
        enabled = schedule?.enabled ?? true
    }

    var startMinute: Int {
        get { windows.first?.startMinute ?? 0 }
        set { windows[0].startMinute = newValue }
    }

    var endMinute: Int {
        get { windows.first?.endMinute ?? 0 }
        set { windows[0].endMinute = newValue }
    }

    var requestWindows: [ActuatorScheduleWindow] {
        windows.map { ActuatorScheduleWindow(startMinute: $0.startMinute, endMinute: $0.endMinute) }
    }

    var canSave: Bool {
        !timezone.isEmpty && windowsAreValid
    }

    mutating func addWindow() {
        guard windows.count < 12 else { return }
        let priorEnd = windows.last?.endMinute ?? 6 * 60
        let start = (priorEnd + 60) % (24 * 60)
        let end = (start + 60) % (24 * 60)
        windows.append(ActuatorScheduleWindowDraft(startMinute: start, endMinute: end))
    }

    private var windowsAreValid: Bool {
        guard !windows.isEmpty, windows.count <= 12 else { return false }
        var occupied = Array(repeating: false, count: 24 * 60)
        for window in windows {
            guard (0..<24 * 60).contains(window.startMinute),
                  (0..<24 * 60).contains(window.endMinute),
                  window.startMinute != window.endMinute else { return false }
            var minute = window.startMinute
            while minute != window.endMinute {
                guard !occupied[minute] else { return false }
                occupied[minute] = true
                minute = (minute + 1) % (24 * 60)
            }
        }
        return true
    }
}

struct ActuatorScheduleWindowDraft: Identifiable, Equatable {
    let id: UUID
    var startMinute: Int
    var endMinute: Int

    init(id: UUID = UUID(), startMinute: Int, endMinute: Int) {
        self.id = id
        self.startMinute = startMinute
        self.endMinute = endMinute
    }

    init(_ window: ActuatorScheduleWindow) {
        self.init(startMinute: window.startMinute, endMinute: window.endMinute)
    }
}

typealias ActuatorScheduleRequest = LightScheduleRequest
typealias ActuatorScheduleDraft = LightScheduleDraft

struct LightStateRequest: Codable, Sendable, Equatable {
    let isOn: Bool
    let actor: String

    enum CodingKeys: String, CodingKey {
        case isOn = "on"
        case actor
    }
}

struct LightStateResponse: Decodable, Sendable {
    let isOn: Bool

    enum CodingKeys: String, CodingKey {
        case isOn = "on"
    }
}

extension Sequence where Element == Actuator {
    func assigned(to plantID: UUID) -> [Actuator] {
        filter { $0.plantIDs.contains(plantID) }
    }
}

enum ActuatorRunDuration: Int, CaseIterable, Identifiable {
    case fiveMinutes = 300
    case tenMinutes = 600
    case fifteenMinutes = 900
    case thirtyMinutes = 1_800
    case oneHour = 3_600

    var id: Int { rawValue }

    var label: String {
        switch self {
        case .fiveMinutes: "5 minutes"
        case .tenMinutes: "10 minutes"
        case .fifteenMinutes: "15 minutes"
        case .thirtyMinutes: "30 minutes"
        case .oneHour: "1 hour"
        }
    }
}

struct ActuatorLease: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let actuatorID: UUID
    let requestedSeconds: Int
    let deadline: Date
    let actor: String
    let source: ObservationSource
    let idempotencyKey: UUID
    let startedAt: Date?
    let stoppedAt: Date?
    let stopReason: String?
    let createdAt: Date

    var isActive: Bool { stoppedAt == nil }

    enum CodingKeys: String, CodingKey {
        case id
        case actuatorID = "actuator_id"
        case requestedSeconds = "requested_seconds"
        case deadline
        case actor
        case source
        case idempotencyKey = "idempotency_key"
        case startedAt = "started_at"
        case stoppedAt = "stopped_at"
        case stopReason = "stop_reason"
        case createdAt = "created_at"
    }
}

struct ActuatorEvent: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let actuatorID: UUID
    let leaseID: UUID?
    let action: ActuatorEventAction
    let actor: String
    let source: ObservationSource
    let idempotencyKey: UUID?
    let detail: String?
    let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case actuatorID = "actuator_id"
        case leaseID = "lease_id"
        case action
        case actor
        case source
        case idempotencyKey = "idempotency_key"
        case detail
        case createdAt = "created_at"
    }
}

struct ActuatorListResponse: Codable, Sendable {
    let actuators: [Actuator]
}

struct ActuatorEventListResponse: Codable, Sendable {
    let events: [ActuatorEvent]
}

struct DiscoveredActuatorListResponse: Codable, Sendable {
    let entities: [HomeAssistantEntity]
}

struct ActuatorRegistration: Codable, Sendable, Equatable {
    let entityID: String
    let name: String
    let kind: ActuatorKind
    let plantIDs: [UUID]

    enum CodingKeys: String, CodingKey {
        case entityID = "entity_id"
        case name
        case kind
        case plantIDs = "plant_ids"
    }
}

struct ActuatorRename: Codable, Sendable {
    let name: String
    let kind: ActuatorKind
    let plantIDs: [UUID]
    let policyControlEnabled: Bool

    enum CodingKeys: String, CodingKey {
        case name
        case kind
        case plantIDs = "plant_ids"
        case policyControlEnabled = "policy_control_enabled"
    }
}

struct ActuatorStartRequest: Codable, Sendable, Equatable {
    let durationSeconds: Int
    let actor: String
    let idempotencyKey: UUID

    enum CodingKeys: String, CodingKey {
        case durationSeconds = "duration_seconds"
        case actor
        case idempotencyKey = "idempotency_key"
    }
}

struct ActuatorStopRequest: Codable, Sendable, Equatable {
    let actor: String
    let idempotencyKey: UUID

    enum CodingKeys: String, CodingKey {
        case actor
        case idempotencyKey = "idempotency_key"
    }
}

struct ActuatorStopResponse: Codable, Sendable {
    let stopped: Bool
}
