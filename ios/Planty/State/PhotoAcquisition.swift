import Foundation
import Observation
import PhotosUI
import SwiftUI

/// A photo as it leaves the shared acquisition layer. Asset id is present for
/// Photos-library imports and lets Snap recover EXIF/GPS from the original.
struct AcquiredPhoto: Sendable, Equatable {
    let jpeg: Data
    let assetID: String?
}

/// Camera permission/session lifecycle and PhotosPicker importing are the same
/// operation everywhere Planty asks for a picture. Screens supply only their
/// framing copy and what to do once bytes arrive.
@Observable
@MainActor
final class PhotoAcquisition {
    let camera: CameraController
    var photoItem: PhotosPickerItem?
    private(set) var error: String?

    @ObservationIgnored
    private let capture: () async throws -> Data

    init(
        camera: CameraController = CameraController(),
        capture: (() async throws -> Data)? = nil
    ) {
        self.camera = camera
        self.capture = capture ?? { try await camera.capture() }
    }

    func prepare() async {
        error = nil
        await camera.prepare()
    }

    func stop() { camera.stop() }

    func takePhoto() async -> AcquiredPhoto? {
        do {
            let jpeg = try await capture()
            error = nil
            return AcquiredPhoto(jpeg: jpeg, assetID: nil)
        } catch {
            self.error = "The camera did not produce a photo. Try again."
            return nil
        }
    }

    func importSelectedPhoto() async -> AcquiredPhoto? {
        guard let item = photoItem else { return nil }
        defer { photoItem = nil }
        do {
            guard let jpeg = try await item.loadTransferable(type: Data.self) else {
                error = "Planty could not read that photo. Try another one."
                return nil
            }
            error = nil
            return AcquiredPhoto(jpeg: jpeg, assetID: item.itemIdentifier)
        } catch {
            error = "Planty could not read that photo. Try another one."
            return nil
        }
    }

    func clearError() { error = nil }
}
