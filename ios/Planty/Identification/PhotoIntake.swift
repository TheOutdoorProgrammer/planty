import Foundation
import Photos

/// A photo the pipeline can work on, however it arrived.
struct IntakePhoto: Sendable, Equatable {
    /// Nil when the library declined or the photo came off the camera, which
    /// means results cannot be cached against an asset.
    var assetID: String?
    var data: Data
    var metadata: CaptureMetadata
}

/// Resolves picker selections to their originals where allowed, and degrades to
/// the sanitised copy where not.
protocol PhotoIntaking: Sendable {
    func intake(pickedData: Data, assetID: String?) async -> IntakePhoto
}

/// Asks Photos for the original bytes. A picker copy commonly has GPS stripped
/// and different EXIF, and capture date and region are two of the few signals
/// that narrow a species set before a model sees anything.
struct PhotoLibraryIntake: PhotoIntaking {
    /// Read-only access is enough and is the smaller ask. `.limited` works too:
    /// the asset resolves if it is one the person actually granted.
    static func authorize() async -> PHAuthorizationStatus {
        let current = PHPhotoLibrary.authorizationStatus(for: .readWrite)
        guard current == .notDetermined else { return current }
        return await PHPhotoLibrary.requestAuthorization(for: .readWrite)
    }

    func intake(pickedData: Data, assetID: String?) async -> IntakePhoto {
        let fallback = IntakePhoto(
            assetID: assetID,
            data: pickedData,
            metadata: CaptureMetadataReader.read(from: pickedData)
        )

        guard let assetID else { return fallback }

        let status = await Self.authorize()
        guard status == .authorized || status == .limited else { return fallback }

        guard let asset = PHAsset.fetchAssets(
            withLocalIdentifiers: [assetID], options: nil
        ).firstObject else { return fallback }

        guard let original = await Self.originalData(for: asset) else { return fallback }

        var metadata = CaptureMetadataReader.read(from: original)
        metadata.fromOriginal = true

        // The asset's own fields beat the file's: EXIF can be absent on an
        // edited copy while Photos still knows when and where it was taken.
        metadata.capturedAt = asset.creationDate ?? metadata.capturedAt
        if let taken = asset.location {
            metadata.latitude = taken.coordinate.latitude
            metadata.longitude = taken.coordinate.longitude
            metadata.altitude = taken.altitude
        }
        metadata.isScreenshot = asset.mediaSubtypes.contains(.photoScreenshot)
            || metadata.isScreenshot

        return IntakePhoto(assetID: assetID, data: original, metadata: metadata)
    }

    /// Returns nil for an iCloud-only asset that will not download. Network
    /// access is opted into explicitly, because without it this silently
    /// returns nil for anything not already on the device.
    private static func originalData(for asset: PHAsset) async -> Data? {
        let options = PHImageRequestOptions()
        options.isNetworkAccessAllowed = true
        options.version = .original
        options.deliveryMode = .highQualityFormat

        return await withCheckedContinuation { continuation in
            PHImageManager.default().requestImageDataAndOrientation(
                for: asset, options: options
            ) { data, _, _, info in
                let cancelled = (info?[PHImageCancelledKey] as? Bool) ?? false
                let failed = info?[PHImageErrorKey] != nil
                continuation.resume(returning: (cancelled || failed) ? nil : data)
            }
        }
    }
}

/// Never touches Photos. What the app falls back to when authorization is
/// declined, and what the simulator uses.
struct PickerOnlyIntake: PhotoIntaking {
    func intake(pickedData: Data, assetID: String?) async -> IntakePhoto {
        IntakePhoto(
            assetID: assetID,
            data: pickedData,
            metadata: CaptureMetadataReader.read(from: pickedData)
        )
    }
}
