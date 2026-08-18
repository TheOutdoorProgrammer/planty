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
    case saving(CapturedPhoto)
    case failed(CapturedPhoto, PlantyError)

    var photo: CapturedPhoto? {
        switch self {
        case .ready: nil
        case .captured(let photo), .saving(let photo), .failed(let photo, _): photo
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
        guard let photo = stage.photo, let plant = selectedPlant else { return }
        stage = .saving(photo)

        do {
            let trimmedNote = note.trimmingCharacters(in: .whitespacesAndNewlines)
            let uploaded = try await api.uploadPhoto(
                slug: plant.slug,
                jpeg: photo.jpeg,
                caption: trimmedNote.isEmpty ? nil : trimmedNote,
                takenAt: photo.takenAt
            )
            if let kind {
                _ = try await api.addObservation(
                    slug: plant.slug,
                    observation: NewObservation(
                        kind: kind,
                        body: trimmedNote.isEmpty ? nil : trimmedNote
                    )
                )
            }
            var saved = photo
            saved.uploaded = uploaded
            stage = .ready
            note = ""
            toast = "Photo added to \(plant.commonName)'s story."
        } catch {
            stage = .failed(photo, PlantyError.from(error))
        }
    }

    func retrySave(recording kind: ObservationKind?) async {
        guard case .failed(let photo, _) = stage else { return }
        stage = .captured(photo)
        await save(recording: kind)
    }

    func clearToast() { toast = nil }
}
