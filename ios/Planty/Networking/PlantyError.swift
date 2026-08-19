import Foundation

/// Every failure the app can hit, phrased so a screen can decide what to say
/// without re-inspecting a URLError code.
enum PlantyError: Error, Sendable, Equatable {
    case notConfigured
    case unauthorized
    case notFound
    case offline
    case timedOut
    case server(status: Int, message: String?)
    case decoding(String)
    case transport(String)

    /// Work thrown away, not work that failed. Its own case because the client
    /// converts at the boundary, and `.transport("cancelled")` could only be
    /// recognised by matching a localised string.
    case cancelled

    /// The record was written but the acknowledgement was not, so the service
    /// is still chasing this. Its own case because the honest thing to say is
    /// neither "saved" nor "failed".
    case stillAsking

    /// True when retrying might work and nothing is wrong with the plants.
    var isTransient: Bool {
        switch self {
        case .offline, .timedOut, .transport, .cancelled, .stillAsking: true
        case .server(let status, _): status >= 500
        default: false
        }
    }
}

extension PlantyError: LocalizedError {
    var errorDescription: String? {
        switch self {
        case .notConfigured:
            "Planty does not know where its service lives yet."
        case .unauthorized:
            "The service refused this token."
        case .notFound:
            "The service has no record of that."
        case .offline:
            "No connection."
        case .timedOut:
            "The service did not answer in time."
        case .server(let status, let message):
            message ?? "The service returned an error (\(status))."
        case .decoding(let detail):
            "The service sent something Planty could not read. \(detail)"
        case .transport(let detail):
            detail
        case .cancelled:
            "Interrupted."
        case .stillAsking:
            "Recorded, but Planty could not stop the reminder. It may ask again."
        }
    }

    /// SwiftUI cancels a `.refreshable` task when the gesture ends, so a good
    /// refresh arrives as an error and must never be shown as a failed check.
    static func isCancellation(_ error: any Error) -> Bool {
        if case .cancelled = PlantyError.from(error) { return true }
        return false
    }

    static func from(_ error: any Error) -> PlantyError {
        if let planty = error as? PlantyError { return planty }
        if error is CancellationError { return .cancelled }
        if let decoding = error as? DecodingError {
            return .decoding(String(describing: decoding))
        }
        let urlError = error as? URLError
        switch urlError?.code {
        case .cancelled:
            return .cancelled
        case .notConnectedToInternet, .networkConnectionLost, .dataNotAllowed:
            return .offline
        case .timedOut:
            return .timedOut
        default:
            return .transport(error.localizedDescription)
        }
    }
}
