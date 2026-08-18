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

    /// True when retrying might work and nothing is wrong with the plants.
    var isTransient: Bool {
        switch self {
        case .offline, .timedOut, .transport: true
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
        }
    }

    /// Work thrown away, not work that failed. SwiftUI cancels a `.refreshable`
    /// task when the gesture ends, so a good refresh arrives here as an error.
    static func isCancellation(_ error: any Error) -> Bool {
        if error is CancellationError { return true }
        if let urlError = error as? URLError { return urlError.code == .cancelled }
        return false
    }

    static func from(_ error: any Error) -> PlantyError {
        if let planty = error as? PlantyError { return planty }
        if let decoding = error as? DecodingError {
            return .decoding(String(describing: decoding))
        }
        let urlError = error as? URLError
        switch urlError?.code {
        case .notConnectedToInternet, .networkConnectionLost, .dataNotAllowed:
            return .offline
        case .timedOut:
            return .timedOut
        default:
            return .transport(error.localizedDescription)
        }
    }
}
