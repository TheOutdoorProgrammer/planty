import Foundation
import Observation

/// A photo that exists on the phone but may not exist on the service yet.
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

/// Where the capture flow is. `captured` keeps holding the image after a failed
/// save, because losing the photo is the one unrecoverable outcome here.
enum CaptureStage: Sendable, Equatable {
    case ready
    case captured(CapturedPhoto)
    case saving(CapturedPhoto, ObservationKind?)
    case failed(CapturedPhoto, ObservationKind?, PlantyError)

    var photo: CapturedPhoto? {
        switch self {
        case .ready: nil
        case .captured(let photo), .saving(let photo, _), .failed(let photo, _, _): photo
        }
    }

    /// What the user said they did, kept across a failure so a retry records
    /// the watering rather than just re-saving the picture.
    var recording: ObservationKind? {
        switch self {
        case .ready, .captured: nil
        case .saving(_, let kind), .failed(_, let kind, _): kind
        }
    }

    var isBusy: Bool {
        if case .saving = self { return true }
        return false
    }
}

@Observable
@MainActor
final class CaptureStore {
    private(set) var stage = CaptureStage.ready
    private(set) var toast: String?

    var selectedPlant: Plant?
    var note = ""

    /// Set when the service is not confident which plant this is, which must
    /// never be resolved silently.
    private(set) var suggestion: Plant?

    /// The verdict this capture was started from, when it came from a card on
    /// Today. Acknowledging it is what stops the service escalating, so a
    /// capture that answers a card has to carry it all the way through.
    var answering: UUID?

    /// Set when the record was written but the verdict could not be settled,
    /// so the card is honestly left in place rather than hidden.
    private(set) var stillAsking = false

    /// The verdict settled by the last successful save, for Today to hide.
    private(set) var settled: UUID?

    private var api: any PlantyAPI

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
    }

    func accept(jpeg: Data) {
        stage = .captured(CapturedPhoto(jpeg: jpeg))
    }

    func retake() {
        stage = .ready
        note = ""
    }

    func discard() {
        stage = .ready
        note = ""
        toast = nil
    }

    /// Saves the photo, then the optional exception tag. The photo goes first
    /// on purpose: an orphan photo is recoverable, a lost one is not.
    func save(recording kind: ObservationKind?) async {
        guard var photo = stage.photo, let plant = selectedPlant else { return }
        stage = .saving(photo, kind)
        stillAsking = false

        let trimmedNote = note.trimmingCharacters(in: .whitespacesAndNewlines)
        do {
            // Skipped when a previous attempt already got the photo up, so a
            // half-failed save does not leave two copies in the story.
            if photo.uploaded == nil {
                photo.uploaded = try await api.uploadPhoto(
                    slug: plant.slug,
                    jpeg: photo.jpeg,
                    caption: trimmedNote.isEmpty ? nil : trimmedNote,
                    takenAt: photo.takenAt
                )
            }
            if let kind {
                _ = try await api.addObservation(
                    slug: plant.slug,
                    observation: NewObservation(
                        kind: kind,
                        body: trimmedNote.isEmpty ? nil : trimmedNote
                    )
                )
            }
        } catch {
            stage = .failed(photo, kind, PlantyError.from(error))
            return
        }

        // Doing the thing the card asked for is what settles it. Without this
        // the card survives the whole flow and the service keeps escalating.
        if let verdict = answering, kind != nil {
            do {
                try await api.acknowledge(verdictID: verdict)
                settled = verdict
            } catch {
                stillAsking = true
            }
        }

        stage = .ready
        note = ""
        answering = nil
        toast = savedMessage(plant: plant, kind: kind)
    }

    private func savedMessage(plant: Plant, kind: ObservationKind?) -> String {
        if stillAsking {
            return "Saved, but Planty may ask again."
        }
        guard let kind else { return "Photo added to \(plant.commonName)'s story." }
        return "\(kind.label) recorded for \(plant.commonName)."
    }

    func retrySave() async {
        guard case .failed(let photo, let kind, _) = stage else { return }
        stage = .captured(photo)
        await save(recording: kind)
    }

    func clearSettled() { settled = nil }

    /// Uploads the photo and hands back the stored record, so a diagnosis can
    /// name the frame it is about. Without this the model was asked to compare
    /// a picture the service had never received.
    func upload() async -> Photo? {
        guard var photo = stage.photo, let plant = selectedPlant else { return nil }
        if let already = photo.uploaded { return already }

        stage = .saving(photo, nil)
        let trimmedNote = note.trimmingCharacters(in: .whitespacesAndNewlines)
        do {
            let uploaded = try await api.uploadPhoto(
                slug: plant.slug,
                jpeg: photo.jpeg,
                caption: trimmedNote.isEmpty ? nil : trimmedNote,
                takenAt: photo.takenAt
            )
            photo.uploaded = uploaded
            stage = .captured(photo)
            return uploaded
        } catch {
            stage = .failed(photo, nil, PlantyError.from(error))
            return nil
        }
    }

    func clearToast() { toast = nil }
}
