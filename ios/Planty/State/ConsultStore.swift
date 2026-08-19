import Foundation
import Observation

/// One side of a conversation about a plant.
struct ConsultMessage: Identifiable, Sendable, Hashable {
    enum Speaker: Sendable { case user, planty }

    let id = UUID()
    let speaker: Speaker
    let text: String
    var answer: PlantAnswer?
}

/// A conversation about a plant's record. Unlike a diagnosis it needs no
/// photograph to start, which is the whole reason it exists: most questions
/// are not about a picture, and demanding one to ask anything is friction.
@Observable
@MainActor
final class ConsultStore {
    private(set) var messages: [ConsultMessage] = []
    private(set) var isThinking = false
    private(set) var error: PlantyError?

    let plant: Plant
    var composer = ""

    private let api: any PlantyAPI
    private var conversationID: UUID?

    init(api: any PlantyAPI, plant: Plant) {
        self.api = api
        self.plant = plant
    }

    /// Openers worth tapping rather than typing, chosen so each is answerable
    /// from the record alone.
    var openers: [String] {
        [
            "Does this need water?",
            "How has it been doing lately?",
            "Am I looking after this one right?"
        ]
    }

    var suggestedFollowUps: [String] {
        messages.reversed().compactMap(\.answer).first?.suggestedFollowUps ?? []
    }

    var hasStarted: Bool { !messages.isEmpty }

    func send() async {
        let text = composer.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty, !isThinking else { return }
        composer = ""
        await ask(text)
    }

    func send(_ prompt: String) async {
        guard !isThinking else { return }
        composer = ""
        await ask(prompt)
    }

    private func ask(_ text: String) async {
        messages.append(ConsultMessage(speaker: .user, text: text))
        isThinking = true
        error = nil
        defer { isThinking = false }

        do {
            let answer = try await api.ask(
                slug: plant.slug,
                question: PlantQuestion(message: text, conversationID: conversationID)
            )
            conversationID = answer.conversationID
            messages.append(
                ConsultMessage(speaker: .planty, text: answer.reply, answer: answer)
            )
        } catch {
            // Leaving the question on screen with no answer under it reads as a
            // reply that has not arrived, so the failure is said out loud.
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
    }

    func clearError() { error = nil }
}
