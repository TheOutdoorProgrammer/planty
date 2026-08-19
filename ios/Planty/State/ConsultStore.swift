import Foundation
import Observation

/// One side of a conversation. The photograph is kept as bytes rather than a
/// stored record: nothing here is filed against a plant.
struct ConsultMessage: Identifiable, Sendable, Hashable {
    enum Speaker: Sendable { case user, planty }

    let id = UUID()
    let speaker: Speaker
    let text: String
    var photo: Data?
    var answer: PlantAnswer?
}

/// One send: the words and the picture that were meant to go together. Kept as
/// a pair so a failure can never hand back the words and drop the photo.
struct ConsultAttempt: Sendable, Hashable {
    var text: String
    var photo: Data?

    var isEmpty: Bool { text.isEmpty && photo == nil }
}

/// A conversation with Planty. With a plant it reads that plant's record; with
/// none it is a scratch chat that creates nothing and files nothing.
@Observable
@MainActor
final class ConsultStore {
    private(set) var messages: [ConsultMessage] = []
    private(set) var isThinking = false
    private(set) var error: PlantyError?

    /// The send that did not land, words and photo together, so it can be
    /// retried or handed back rather than reassembled from memory.
    private(set) var failed: ConsultAttempt?

    /// Hanging off the next message, not yet sent anywhere.
    private(set) var attachment: Data?

    /// Nil is the scratch chat: it asks /v1/ask, which names no plant, creates
    /// none, and writes to no timeline.
    let plant: Plant?
    var composer = ""

    private let api: any PlantyAPI
    private var conversationID: UUID?

    /// Asked the moment the screen opens, for entry points that already know
    /// the question. The toxicity card's "is this dangerous" is the one caller.
    private var pending: String?

    init(
        api: any PlantyAPI,
        plant: Plant?,
        attachment: Data? = nil,
        pending: String? = nil
    ) {
        self.api = api
        self.plant = plant
        self.attachment = attachment
        self.pending = pending
    }

    var title: String {
        guard let plant else { return "Ask Planty" }
        return "Ask about \(plant.commonName)"
    }

    /// Openers worth tapping rather than typing. With a plant they are
    /// answerable from its record; without one they are about the picture.
    var openers: [String] {
        guard plant != nil else {
            return [
                "What is this?",
                "Is this safe around cats and dogs?",
                "Is something wrong with it?"
            ]
        }
        return [
            "Does this need water?",
            "How has it been doing lately?",
            "Am I looking after this one right?"
        ]
    }

    var thinkingLine: String {
        guard let plant else { return "Looking at what you sent." }
        return "Reading \(plant.commonName)'s record."
    }

    var suggestedFollowUps: [String] {
        messages.reversed().compactMap(\.answer).first?.suggestedFollowUps ?? []
    }

    var hasStarted: Bool { !messages.isEmpty }

    /// A picture on its own is a question, so an empty composer is sendable
    /// whenever something is attached.
    var canSend: Bool {
        guard !isThinking else { return false }
        return !composer.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
            || attachment != nil
    }

    func attach(jpeg: Data) { attachment = jpeg }

    func removeAttachment() { attachment = nil }

    /// Fires the question this store was opened with, once.
    func begin() async {
        guard let question = pending, messages.isEmpty else { return }
        pending = nil
        await send(question)
    }

    func send() async {
        guard canSend else { return }
        let attempt = ConsultAttempt(
            text: composer.trimmingCharacters(in: .whitespacesAndNewlines),
            photo: attachment
        )
        composer = ""
        attachment = nil
        await ask(attempt)
    }

    /// A suggestion never destroys a half-typed question: tapping one while
    /// something is in the box is an accident waiting to lose it.
    func send(_ prompt: String) async {
        guard !isThinking else { return }
        let draft = composer
        let pendingPhoto = attachment
        composer = ""
        attachment = nil
        await ask(ConsultAttempt(text: prompt, photo: pendingPhoto))
        if failed != nil { composer = draft }
    }

    /// Sends the same words and the same picture again, and clears the dangling
    /// bubble that has no answer under it.
    func retry() async {
        guard let attempt = failed else { return }
        failed = nil
        if case .user = messages.last?.speaker { messages.removeLast() }
        await ask(attempt)
    }

    /// Drops the failed attempt and hands both halves back: the words to the
    /// composer, the photo to the attachment slot. Losing the photo here is the
    /// one outcome that cannot be undone from the app.
    func recoverDraft() {
        guard let attempt = failed else { return }
        if case .user = messages.last?.speaker { messages.removeLast() }
        composer = attempt.text
        attachment = attempt.photo
        failed = nil
        error = nil
    }

    private func ask(_ attempt: ConsultAttempt) async {
        guard !attempt.isEmpty else { return }
        messages.append(
            ConsultMessage(speaker: .user, text: attempt.text, photo: attempt.photo)
        )
        isThinking = true
        error = nil
        defer { isThinking = false }

        do {
            let reply = try await answer(to: attempt)
            conversationID = reply.conversationID
            failed = nil
            messages.append(
                ConsultMessage(speaker: .planty, text: reply.reply, answer: reply)
            )
        } catch {
            // Leaving the question on screen with no answer under it reads as a
            // reply that has not arrived, so the failure is said out loud.
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
            failed = attempt
        }
    }

    /// Both endpoints answer in the same shape; which one gets asked is the
    /// only difference between a consultation and a scratch chat.
    private func answer(to attempt: ConsultAttempt) async throws -> PlantAnswer {
        guard let plant else {
            return try await api.ask(
                ScratchQuestion(
                    message: attempt.text.isEmpty ? nil : attempt.text,
                    photo: attempt.photo,
                    conversationID: conversationID
                )
            )
        }

        return try await api.ask(
            slug: plant.slug,
            question: PlantQuestion(
                message: attempt.text,
                photo: attempt.photo,
                conversationID: conversationID
            )
        )
    }

    func clearError() { error = nil }
}
