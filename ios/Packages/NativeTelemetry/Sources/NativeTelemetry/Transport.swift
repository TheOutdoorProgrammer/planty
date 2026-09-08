import Foundation

public final class NativeHTTPTransport: NSObject, NativeTransport, URLSessionTaskDelegate, @unchecked Sendable {
    private let endpoint: URL
    private let bearer: String

    public init?(baseURL: URL, bearer: String) {
        guard baseURL.scheme == "https", baseURL.user == nil, baseURL.password == nil,
              baseURL.query == nil, baseURL.fragment == nil, !bearer.isEmpty else { return nil }
        endpoint = baseURL.appendingPathComponent("v1/native-telemetry")
        self.bearer = bearer
    }

    public func send(_ envelope: NativeEnvelope) async throws -> NativeDelivery {
        let body = try NativeCoding.encoder().encode(envelope)
        guard body.count <= 65_536, (1...16).contains(envelope.events.count) else { return .rejected }
        let configuration = URLSessionConfiguration.ephemeral
        configuration.urlCache = nil
        configuration.httpCookieStorage = nil
        configuration.urlCredentialStorage = nil
        configuration.timeoutIntervalForRequest = 10
        configuration.timeoutIntervalForResource = 10
        let session = URLSession(configuration: configuration, delegate: self, delegateQueue: nil)
        defer { session.invalidateAndCancel() }
        var request = URLRequest(url: endpoint)
        request.httpMethod = "POST"
        request.httpBody = body
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("Bearer \(bearer)", forHTTPHeaderField: "Authorization")
        let (_, response) = try await session.bytes(for: request)
        guard let response = response as? HTTPURLResponse else { return .retry(after: 60) }
        return Self.delivery(for: response, envelope: envelope)
    }

    static func delivery(for response: HTTPURLResponse, envelope: NativeEnvelope) -> NativeDelivery {
        let minimalDiagnostic = envelope.events.contains { event in
            event.crash == nil && event.outcome == .failure
                && ((event.operation == .appCrash && event.errorClass == .crash)
                    || (event.operation == .appHang && event.errorClass == .hang))
        }
        switch response.statusCode {
        case 204: return .accepted
        // Older relays require frames during a rolling upgrade.
        case 400 where minimalDiagnostic: return .retry(after: 60)
        case 400, 413, 422: return .rejected
        default: return .retry(after: Double(response.value(forHTTPHeaderField: "Retry-After") ?? "") ?? 60)
        }
    }

    public func urlSession(
        _ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse,
        newRequest request: URLRequest, completionHandler: @escaping @Sendable (URLRequest?) -> Void
    ) {
        completionHandler(nil)
    }
}
