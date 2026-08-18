import AVFoundation
import Observation
import UIKit

/// AVFoundation types are not Sendable, so crossing them onto the session queue
/// needs an explicit box rather than a silent assumption.
private struct UncheckedBox<T>: @unchecked Sendable {
    let value: T
}

enum CameraAvailability: Sendable, Equatable {
    case unknown
    case ready
    case denied
    /// No capture device at all, which is every simulator.
    case unavailable
}

@Observable
@MainActor
final class CameraController {
    private(set) var availability = CameraAvailability.unknown
    private(set) var isRunning = false

    @ObservationIgnored let session = AVCaptureSession()
    @ObservationIgnored private let output = AVCapturePhotoOutput()
    @ObservationIgnored private let queue = DispatchQueue(label: "zone.stout.planty.camera")
    @ObservationIgnored private var isConfigured = false
    @ObservationIgnored private var delegates: [UUID: PhotoCaptureDelegate] = [:]

    func prepare() async {
        switch AVCaptureDevice.authorizationStatus(for: .video) {
        case .authorized:
            break
        case .notDetermined:
            guard await AVCaptureDevice.requestAccess(for: .video) else {
                availability = .denied
                return
            }
        default:
            availability = .denied
            return
        }

        guard configure() else {
            availability = .unavailable
            return
        }
        availability = .ready
        start()
    }

    func start() {
        guard availability == .ready, !isRunning else { return }
        isRunning = true
        let box = UncheckedBox(value: session)
        queue.async { box.value.startRunning() }
    }

    func stop() {
        guard isRunning else { return }
        isRunning = false
        let box = UncheckedBox(value: session)
        queue.async { box.value.stopRunning() }
    }

    func capture() async throws -> Data {
        guard availability == .ready else { throw CameraError.unavailable }

        let token = UUID()
        let settings = AVCapturePhotoSettings(format: [AVVideoCodecKey: AVVideoCodecType.jpeg])
        let outputBox = UncheckedBox(value: output)

        return try await withCheckedThrowingContinuation { continuation in
            let delegate = PhotoCaptureDelegate { [weak self] result in
                Task { @MainActor in self?.delegates[token] = nil }
                continuation.resume(with: result)
            }
            delegates[token] = delegate
            outputBox.value.capturePhoto(with: settings, delegate: delegate)
        }
    }

    private func configure() -> Bool {
        guard !isConfigured else { return true }
        guard let device = AVCaptureDevice.default(
            .builtInWideAngleCamera,
            for: .video,
            position: .back
        ),
            let input = try? AVCaptureDeviceInput(device: device)
        else { return false }

        session.beginConfiguration()
        session.sessionPreset = .photo
        guard session.canAddInput(input), session.canAddOutput(output) else {
            session.commitConfiguration()
            return false
        }
        session.addInput(input)
        session.addOutput(output)
        session.commitConfiguration()

        isConfigured = true
        return true
    }
}

enum CameraError: Error, Sendable {
    case unavailable
    case noData
}

private final class PhotoCaptureDelegate: NSObject, AVCapturePhotoCaptureDelegate, @unchecked Sendable {
    private let completion: @Sendable (Result<Data, any Error>) -> Void

    init(completion: @escaping @Sendable (Result<Data, any Error>) -> Void) {
        self.completion = completion
    }

    func photoOutput(
        _ output: AVCapturePhotoOutput,
        didFinishProcessingPhoto photo: AVCapturePhoto,
        error: (any Error)?
    ) {
        if let error {
            completion(.failure(error))
            return
        }
        guard let data = photo.fileDataRepresentation() else {
            completion(.failure(CameraError.noData))
            return
        }
        completion(.success(data))
    }
}
