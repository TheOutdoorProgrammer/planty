import Foundation
import Observation

/// The conversation that follows a photo. It never starts empty: there is
/// always a plant and an image behind it, which is why this is not a tab.
@Observable
@MainActor
final class DiagnosisStore {
    private(set) var messages: [DiagnosisMessage] = []
    private(set) var isThinking = false
    private(set) var error: PlantyError?
    private(set) var stageLine: String?

    let plant: Plant
    let photo: CapturedPhoto?
    var composer = ""

    private let service: any DiagnosisService
    private let api: any PlantyAPI
    private var conversationID: UUID?

    init(
        service: any DiagnosisService,
        api: any PlantyAPI,
        plant: Plant,
        photo: CapturedPhoto?
    ) {
        self.service = service
        self.api = api
        self.plant = plant
        self.photo = photo
    }

    var latestTurn: DiagnosisTurn? {
        messages.reversed().compactMap(\.turn).first
    }

    var suggestedFollowUps: [String] {
        latestTurn?.suggestedFollowUps ?? []
    }

    /// Only ever counts photos it actually has, because "Comparing today's
    /// photo with 6 earlier photos" has to be true to be worth saying.
    func begin(comparingAgainst earlierPhotos: Int) async {
        guard messages.isEmpty else { return }
        messages.append(DiagnosisMessage(speaker: .user, text: "Something looks off."))
        stageLine = earlierPhotos > 0
            ? "Comparing today's photo with \(earlierPhotos) earlier photos."
            : "Reading today's photo. There is nothing earlier to compare it with."
        await ask { [service, plant, photo] in
            try await service.open(
                slug: plant.slug,
                photoID: photo?.uploaded?.id,
                prompt: "Something looks off."
            )
        }
    }

    func send() async {
        let text = composer.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return }
        composer = ""
        messages.append(DiagnosisMessage(speaker: .user, text: text))
        stageLine = "Writing the answer."
        let conversation = conversationID ?? UUID()
        await ask { [service, plant] in
            try await service.follow(
                slug: plant.slug,
                conversationID: conversation,
                message: text
            )
        }
    }

    func send(followUp: String) async {
        composer = followUp
        await send()
    }

    /// Offered actions record real observations; the answer is not a substitute
    /// for the record of what the user actually did.
    func perform(_ action: DiagnosisOfferedAction) async {
        guard let kind = action.recordsKind else { return }
        do {
            _ = try await api.addObservation(
                slug: plant.slug,
                observation: NewObservation(kind: kind, body: action.note)
            )
            messages.append(DiagnosisMessage(speaker: .user, text: action.title))
        } catch {
            self.error = PlantyError.from(error)
        }
    }

    func clearError() { error = nil }

    private func ask(_ work: @escaping @Sendable () async throws -> DiagnosisTurn) async {
        isThinking = true
        error = nil
        defer {
            isThinking = false
            stageLine = nil
        }

        do {
            let turn = try await work()
            conversationID = conversationID ?? turn.id
            messages.append(
                DiagnosisMessage(speaker: .planty, text: turn.observed, turn: turn)
            )
        } catch {
            self.error = PlantyError.from(error)
        }
    }
}
