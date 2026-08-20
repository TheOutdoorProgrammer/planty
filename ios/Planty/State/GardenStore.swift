import Foundation
import Observation

/// Cross-plant work that belongs to the garden rather than one plant's story.
@Observable
@MainActor
final class GardenStore {
    private(set) var questions: [OpenQuestion] = []
    private(set) var postmortems: [Postmortem] = []
    private(set) var harvests: [Harvest] = []
    private(set) var awayPeriods: [AwayPeriod] = []
    private(set) var coldWatch: ColdWatch?
    private(set) var plannedAway: AwayPeriod?
    private(set) var error: PlantyError?
    private(set) var isLoading = false
    private(set) var hasLoaded = false

    var questionStatus: QuestionStatus = .open
    var isConfigured: Bool

    private var api: any PlantyAPI
    private var loadID = UUID()

    init(api: any PlantyAPI, isConfigured: Bool) {
        self.api = api
        self.isConfigured = isConfigured
    }

    func replace(api: any PlantyAPI, isConfigured: Bool) {
        self.api = api
        self.isConfigured = isConfigured
        loadID = UUID()
        questions = []
        postmortems = []
        harvests = []
        awayPeriods = []
        coldWatch = nil
        plannedAway = nil
        error = nil
        hasLoaded = false
    }

    func load() async {
        guard isConfigured else { return }
        let requestID = UUID()
        loadID = requestID
        isLoading = true
        error = nil

        let api = self.api
        let status = questionStatus
        async let questionsTask = gardenResult { try await api.questions(status: status) }
        async let postmortemsTask = gardenResult { try await api.postmortems() }
        async let harvestsTask = gardenResult { try await api.harvests(slug: nil) }
        async let awayTask = gardenResult { try await api.awayPeriods() }
        let loaded = await (questionsTask, postmortemsTask, harvestsTask, awayTask)
        guard requestID == loadID else { return }

        if case .success(let value) = loaded.0 { questions = value }
        if case .success(let value) = loaded.1 { postmortems = value }
        if case .success(let value) = loaded.2 { harvests = value }
        if case .success(let value) = loaded.3 {
            awayPeriods = value
            plannedAway = value.first
        }
        error = [loaded.0.failure, loaded.1.failure, loaded.2.failure, loaded.3.failure]
            .compactMap { $0 }
            .first { !PlantyError.isCancellation($0) }
        hasLoaded = true
        isLoading = false
    }

    func loadQuestions() async {
        guard isConfigured else { return }
        do {
            questions = try await api.questions(status: questionStatus)
        } catch {
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
    }

    func createQuestion(_ draft: NewOpenQuestion) async -> PlantyError? {
        do {
            _ = try await api.createQuestion(draft)
            questionStatus = .open
            await loadQuestions()
            return error
        } catch {
            return PlantyError.from(error)
        }
    }

    func answer(_ question: OpenQuestion, with words: String) async -> PlantyError? {
        let answer = words.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !answer.isEmpty else { return nil }
        do {
            try await api.answerQuestion(id: question.id, answer: answer)
            await loadQuestions()
            return error
        } catch {
            return PlantyError.from(error)
        }
    }

    func planAway(_ draft: NewAwayPeriod) async -> PlantyError? {
        do {
            let saved = try await api.planAway(draft)
            awayPeriods.append(saved)
            awayPeriods.sort { $0.startsAt < $1.startsAt }
            plannedAway = awayPeriods.first
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    func updateAway(id: UUID, draft: NewAwayPeriod) async -> PlantyError? {
        do {
            let saved = try await api.updateAway(id: id, draft: draft)
            if let index = awayPeriods.firstIndex(where: { $0.id == id }) {
                awayPeriods[index] = saved
            } else {
                awayPeriods.append(saved)
            }
            awayPeriods.sort { $0.startsAt < $1.startsAt }
            plannedAway = awayPeriods.first
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    func cancelAway(id: UUID) async -> PlantyError? {
        do {
            try await api.cancelAway(id: id)
            awayPeriods.removeAll { $0.id == id }
            plannedAway = awayPeriods.first
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    func checkCold(forecastLowF: Double) async -> PlantyError? {
        do {
            coldWatch = try await api.coldWatch(forecastLowF: forecastLowF)
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }

    func ownerUpdate(steward: String) async throws -> OwnerUpdate {
        try await api.ownerUpdate(steward: steward)
    }

    func clearError() { error = nil }
}

private func gardenResult<Value: Sendable>(
    _ operation: @escaping @Sendable () async throws -> Value
) async -> Result<Value, PlantyError> {
    do {
        return .success(try await operation())
    } catch {
        return .failure(PlantyError.from(error))
    }
}

private extension Result where Failure == PlantyError {
    var failure: PlantyError? {
        guard case .failure(let error) = self else { return nil }
        return error
    }
}
