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

    /// Uploads today's photo and answers with its stored id. Injected because
    /// the capture flow owns the image and its upload state.
    private let uploader: (@MainActor (CapturedPhoto) async -> UUID?)?

    init(
        service: any DiagnosisService,
        api: any PlantyAPI,
        plant: Plant,
        photo: CapturedPhoto?,
        uploader: (@MainActor (CapturedPhoto) async -> UUID?)? = nil
    ) {
        self.service = service
        self.api = api
        self.plant = plant
        self.photo = photo
        self.uploader = uploader
    }

    var latestTurn: DiagnosisTurn? {
        messages.reversed().compactMap(\.turn).first
    }

    var suggestedFollowUps: [String] {
        latestTurn?.suggestedFollowUps ?? []
    }

    /// Only ever counts photos it actually has, because "Comparing today's
    /// photo with 6 earlier photos" has to be true to be worth saying.
    ///
    /// The count is read here rather than passed in: the caller used to hand
    /// over a hardcoded zero, so every diagnosis opened by claiming there was
    /// nothing earlier to compare against, for a product whose whole job is
    /// the comparison.
    func begin() async {
        guard messages.isEmpty else { return }
        messages.append(DiagnosisMessage(speaker: .user, text: "Something looks off."))
        isThinking = true
        stageLine = "Sending today's photo."

        let uploadedID = await uploadIfNeeded()
        let earlier = await countEarlierPhotos()
        isThinking = false

        stageLine = earlier > 0
            ? "Comparing today's photo with \(earlier) earlier \(earlier == 1 ? "photo" : "photos")."
            : "Reading today's photo. There is nothing earlier to compare it with."

        await ask { [service, plant] in
            try await service.open(
                slug: plant.slug,
                photoID: uploadedID,
                prompt: "Something looks off."
            )
        }
    }

    /// The model is asked about a specific frame, so that frame has to exist
    /// on the service. A failure here is not fatal: the answer degrades to the
    /// stored timeline rather than not arriving.
    private func uploadIfNeeded() async -> UUID? {
        if let already = photo?.uploaded?.id { return already }
        guard let photo, let uploader else { return nil }
        return await uploader(photo)
    }

    private func countEarlierPhotos() async -> Int {
        guard let timeline = try? await api.timeline(slug: plant.slug) else { return 0 }
        let todays = photo?.uploaded?.id
        return timeline.photos.filter { $0.id != todays }.count
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
