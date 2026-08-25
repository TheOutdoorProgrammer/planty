import Foundation

/// The species backend, kept behind a protocol so it is swappable without
/// touching a call site.
protocol PlantIdentifying: Sendable {
    /// Names this backend in a stored result, so an answer stays attributable
    /// when the backend changes underneath it.
    var backendID: String { get }

    func identify(
        imageData: Data,
        metadata: CaptureMetadata
    ) async throws -> [IdentificationCandidate]
}

/// Planty's own service answers, so the credential stays server side: a
/// species-API key in a shipped binary belongs to anyone who unzips the app.
struct RemotePlantIdentifier: PlantIdentifying {
    let api: any PlantyAPI
    let serviceID: String

    var backendID: String { "planty/v1@\(serviceID)" }

    func identify(
        imageData: Data,
        metadata: CaptureMetadata
    ) async throws -> [IdentificationCandidate] {
        try await api.identify(jpeg: imageData, metadata: metadata)
    }
}

/// Answers nothing, on purpose. What the app uses when no service is
/// configured, so the pipeline still runs its offline half.
struct UnavailableIdentifier: PlantIdentifying {
    var backendID: String { "unavailable" }

    func identify(imageData: Data, metadata: CaptureMetadata) async throws -> [IdentificationCandidate] {
        throw PlantyError.notConfigured
    }
}
