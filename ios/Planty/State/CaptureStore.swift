import Foundation
import Observation

struct CapturedPhoto: Sendable, Equatable, Identifiable {
    let id: UUID
    let jpeg: Data
    let takenAt: Date
    var uploaded: Photo?

    init(jpeg: Data, takenAt: Date = Date()) {
        id = UUID()
        self.jpeg = jpeg
        self.takenAt = takenAt
    }
}

enum CaptureStage: Sendable, Equatable {
    case ready
    case captured(CapturedPhoto)
    case saving(CapturedPhoto, ObservationKind?)
    case failed(CapturedPhoto, FailedCaptureAction, PlantyError)

    var photo: CapturedPhoto? {
        switch self {
        case .ready: nil
        case .captured(let photo), .saving(let photo, _), .failed(let photo, _, _): photo
        }
    }

    var recording: ObservationKind? {
        switch self {
        case .ready, .captured: nil
        case .saving(_, let kind): kind
        case .failed(_, .save(let kind), _): kind
        case .failed(_, .create, _): nil
        }
    }

    var isBusy: Bool {
        if case .saving = self { return true }
        return false
    }
}

enum FailedCaptureAction: Sendable, Equatable {
    case save(ObservationKind?)
    case create(name: String?, metadata: CaptureMetadata)
}

@Observable
@MainActor
final class CaptureStore {
    private(set) var stage = CaptureStage.ready
    private(set) var toast: String?
    var selectedPlant: Plant?
    var note = ""
    private(set) var suggestion: Plant?
    var answering: UUID?
    private(set) var stillAsking = false
    private(set) var settled: UUID?
    private var api: any PlantyAPI
    private var completionAttempt: CaptureCompletionAttempt?

    init(api: any PlantyAPI, selectedPlant: Plant? = nil) {
        self.api = api
        self.selectedPlant = selectedPlant
    }

    func replace(api: any PlantyAPI) {
        self.api = api
        stage = .ready
        selectedPlant = nil
        note = ""
        answering = nil
        settled = nil
        stillAsking = false
        completionAttempt = nil
    }

    func accept(jpeg: Data) {
        completionAttempt = nil
        stage = .captured(CapturedPhoto(jpeg: jpeg))
    }

    func retake() {
        stage = .ready
        note = ""
        completionAttempt = nil
    }

    func discard() {
        stage = .ready
        note = ""
        toast = nil
        completionAttempt = nil
    }

    func save(recording kind: ObservationKind?) async {
        guard var photo = stage.photo, let plant = selectedPlant else { return }
        stage = .saving(photo, kind)
        stillAsking = false
        let trimmedNote = note.trimmingCharacters(in: .whitespacesAndNewlines)

        do {
            if photo.uploaded == nil {
                photo.uploaded = try await api.uploadPhoto(
                    slug: plant.slug,
                    jpeg: photo.jpeg,
                    caption: trimmedNote.isEmpty ? nil : trimmedNote,
                    takenAt: photo.takenAt
                )
            }

            if let kind {
                if let verdict = try await verdictToSettle(for: plant) {
                    let identity = CaptureCompletionIdentity(
                        verdictID: verdict,
                        kind: kind,
                        note: trimmedNote
                    )
                    let attempt: CaptureCompletionAttempt
                    if let pending = completionAttempt, pending.identity == identity {
                        attempt = pending
                    } else {
                        attempt = CaptureCompletionAttempt(id: UUID(), identity: identity)
                        completionAttempt = attempt
                    }
                    _ = try await api.completeVerdict(
                        slug: plant.slug,
                        verdictID: verdict,
                        kind: kind,
                        note: trimmedNote,
                        idempotencyKey: attempt.id
                    )
                    settled = verdict
                    completionAttempt = nil
                } else {
                    _ = try await api.addObservation(
                        slug: plant.slug,
                        observation: NewObservation(
                            kind: kind,
                            body: trimmedNote.isEmpty ? nil : trimmedNote
                        )
                    )
                }
            }
        } catch {
            stage = .failed(photo, .save(kind), PlantyError.from(error))
            return
        }

        stage = .ready
        note = ""
        answering = nil
        toast = savedMessage(plant: plant, kind: kind)
    }

    private func verdictToSettle(for plant: Plant) async throws -> UUID? {
        if let answering { return answering }
        let verdict = try await api.plant(slug: plant.slug).verdict
        guard let verdict, verdict.needsAction, !verdict.isAcknowledged else { return nil }
        return verdict.id
    }

    private func savedMessage(plant: Plant, kind: ObservationKind?) -> String {
        guard let kind else { return "Photo added to \(plant.commonName)'s story." }
        return "\(kind.label) recorded for \(plant.commonName)."
    }

    func retrySave() async -> Plant? {
        guard case .failed(let photo, let action, _) = stage else { return nil }
        stage = .captured(photo)
        switch action {
        case .save(let kind):
            await save(recording: kind)
            return nil
        case .create(let name, let metadata):
            return await createPlant(named: name, metadata: metadata)
        }
    }

    func clearSettled() { settled = nil }

    func createPlant(named name: String?, metadata: CaptureMetadata) async -> Plant? {
        guard let photo = stage.photo else { return nil }
        stage = .saving(photo, nil)

        do {
            let made = try await api.createPlantFromPhoto(
                PlantFromPhoto(
                    jpeg: photo.jpeg,
                    metadata: metadata,
                    commonName: name,
                    location: nil,
                    steward: nil
                )
            )
            selectedPlant = made.plant
            stage = .ready
            note = ""
            toast = made.photoError == nil
                ? "\(made.plant.commonName) added, with today's photo."
                : "\(made.plant.commonName) added, but the photo was not kept."
            return made.plant
        } catch {
            stage = .failed(
                photo,
                .create(name: name, metadata: metadata),
                PlantyError.from(error)
            )
            return nil
        }
    }

    func clearToast() { toast = nil }
}

private struct CaptureCompletionIdentity: Equatable {
    let verdictID: UUID
    let kind: ObservationKind
    let note: String
}

private struct CaptureCompletionAttempt {
    let id: UUID
    let identity: CaptureCompletionIdentity
}
