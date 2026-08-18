import CoreImage
import Foundation
import Vision

/// The Vision half of the pipeline, behind a protocol so the pipeline tests
/// without a GPU or a real image.
protocol ImageAnalyzing: Sendable {
    /// Cuts the subject out of its background, or returns nil when Vision finds
    /// no foreground instance.
    func isolateSubject(in data: Data) async throws -> Data?

    func classify(_ data: Data) async throws -> [CoarseLabel]
}

/// Vision on real bytes. Everything takes and returns `Data` so no CGImage
/// crosses an isolation boundary.
struct VisionImageAnalyzer: ImageAnalyzing {
    /// Indoor plant photos are cluttered with pots, furniture and walls, and
    /// classifying the raw frame drags accuracy down badly. Cropping to the
    /// instance extent is what makes the cutout worth classifying.
    func isolateSubject(in data: Data) async throws -> Data? {
        let handler = ImageRequestHandler(data)
        let request = GenerateForegroundInstanceMaskRequest()

        guard let observation = try await handler.perform(request),
              !observation.allInstances.isEmpty
        else { return nil }

        let masked = try observation.generateMaskedImage(
            for: observation.allInstances,
            imageFrom: handler,
            croppedToInstancesExtent: true
        )
        return Self.encode(CIImage(cvPixelBuffer: masked))
    }

    func classify(_ data: Data) async throws -> [CoarseLabel] {
        let observations = try await ClassifyImageRequest().perform(on: data)
        return observations.map {
            CoarseLabel(identifier: $0.identifier, confidence: Double($0.confidence))
        }
    }

    /// JPEG rather than PNG: the masked buffer goes on to a network call, and
    /// a lossless cutout of a houseplant is a pointless megabyte.
    private static func encode(_ image: CIImage) -> Data? {
        let context = CIContext()
        return context.jpegRepresentation(
            of: image,
            colorSpace: image.colorSpace ?? CGColorSpaceCreateDeviceRGB(),
            options: [kCGImageDestinationLossyCompressionQuality as CIImageRepresentationOption: 0.9]
        )
    }
}
