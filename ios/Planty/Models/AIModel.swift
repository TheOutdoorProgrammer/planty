import Foundation

/// What a model can do. The server decides this from what it has actually
/// observed each model doing, so the app never infers capability from a name.
struct AIModelSkills: Codable, Sendable, Hashable {
    let vision: Bool
    let schema: Bool
    let tools: Bool
}

/// One selectable model. `jobs` is the server's own list of what this model may
/// be assigned to, which is what the picker filters on rather than re-deriving
/// the rules on the phone.
struct AIModel: Codable, Sendable, Hashable, Identifiable {
    let provider: String
    let modelID: String
    let ref: String
    let name: String
    let rank: Int
    let skills: AIModelSkills
    let note: String?
    let jobs: [AIJob]

    var id: String { ref }

    func can(_ job: AIJob) -> Bool { jobs.contains(job) }

    enum CodingKeys: String, CodingKey {
        case provider
        case modelID = "id"
        case ref
        case name
        case rank
        case skills
        case note
        case jobs
    }
}

/// Which model answers one job, and whether that is a deliberate choice or the
/// service's own default.
struct JobAssignment: Codable, Sendable, Hashable, Identifiable {
    let job: AIJob
    let provider: String?
    let model: String?
    let ref: String?
    let isDefault: Bool

    var id: AIJob { job }

    enum CodingKeys: String, CodingKey {
        case job
        case provider
        case model
        case ref
        case isDefault = "default"
    }
}

struct AIModelList: Codable, Sendable {
    let models: [AIModel]
}

struct JobAssignmentList: Codable, Sendable {
    let assignments: [JobAssignment]
}

struct ModelAssignmentRequest: Codable, Sendable {
    let provider: String
    let model: String
}
