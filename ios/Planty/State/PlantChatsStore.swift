import Foundation
import Observation

@Observable
@MainActor
final class PlantChatsStore {
    private(set) var conversations: [PlantConversationSummary] = []
    private(set) var isLoading = false
    private(set) var loadingConversationID: UUID?
    private(set) var error: PlantyError?

    let plant: Plant
    private let api: any PlantyAPI

    init(api: any PlantyAPI, plant: Plant) {
        self.api = api
        self.plant = plant
    }

    func load() async {
        guard !isLoading else { return }
        isLoading = true
        error = nil
        defer { isLoading = false }

        do {
            conversations = try await api.conversations(slug: plant.slug)
        } catch {
            if PlantyError.isCancellation(error) { return }
            self.error = PlantyError.from(error)
        }
    }

    func resume(_ summary: PlantConversationSummary) async -> Result<ConsultStore, PlantyError> {
        guard loadingConversationID == nil else {
            return .failure(.server(status: 409, message: "Another chat is already opening."))
        }
        loadingConversationID = summary.id
        error = nil
        defer { loadingConversationID = nil }

        do {
            let conversation = try await api.conversation(slug: plant.slug, id: summary.id)
            return .success(ConsultStore(api: api, plant: plant, conversation: conversation))
        } catch {
            let failure = PlantyError.from(error)
            if PlantyError.isCancellation(failure) { return .failure(.cancelled) }
            self.error = failure
            return .failure(failure)
        }
    }
}
