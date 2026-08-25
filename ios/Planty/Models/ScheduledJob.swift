import Foundation

enum ScheduledJobID: String, FallbackDecodable, CaseIterable {
    case ingest
    case verifyWater = "verify-water"
    case reconcileActuators = "reconcile-actuators"
    case prunePhotos = "prune-photos"
    case daily
    case chase
    case away
    case thirst
    case cold
    case remind
    case unknown

    static let fallback = ScheduledJobID.unknown
}

enum ScheduledJobRunState: String, FallbackDecodable, CaseIterable {
    case queued
    case running
    case succeeded
    case failed
    case unknown

    static let fallback = ScheduledJobRunState.unknown

    var isActive: Bool { self == .queued || self == .running }
    var isTerminal: Bool { self == .succeeded || self == .failed }
}

struct ScheduledJobRun: Codable, Sendable, Hashable, Identifiable {
    let id: String
    let job: ScheduledJobID
    let state: ScheduledJobRunState
    let startedAt: Date?
    let completedAt: Date?
    let detail: String?

    enum CodingKeys: String, CodingKey {
        case id, job, state, detail
        case startedAt = "started_at"
        case completedAt = "completed_at"
    }
}

struct ScheduledJob: Codable, Sendable, Hashable, Identifiable {
    let id: ScheduledJobID
    let name: String
    let purpose: String
    let category: String
    let cadence: String
    let schedule: String
    let timeZone: String
    let suspended: Bool
    var latestRun: ScheduledJobRun?

    enum CodingKeys: String, CodingKey {
        case id, name, purpose, category, cadence, schedule, suspended
        case timeZone = "time_zone"
        case latestRun = "latest_run"
    }
}

struct ScheduledJobListResponse: Codable, Sendable {
    let jobs: [ScheduledJob]
}
