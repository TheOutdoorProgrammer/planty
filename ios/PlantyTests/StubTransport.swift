import Foundation

@testable import Planty

/// Answers for the stubbed protocol. A locked box rather than a global var,
/// because Swift Testing runs suites concurrently.
final class StubResponder: @unchecked Sendable {
    typealias Handler = @Sendable (URLRequest) throws -> (HTTPURLResponse, Data)

    static let shared = StubResponder()

    private let lock = NSLock()
    private var handler: Handler?
    private var seen: [URLRequest] = []

    func install(_ handler: @escaping Handler) {
        lock.withLock {
            self.handler = handler
            seen = []
        }
    }

    func respond(to request: URLRequest) throws -> (HTTPURLResponse, Data) {
        let handler = lock.withLock {
            seen.append(request)
            return self.handler
        }
        guard let handler else { throw URLError(.badServerResponse) }
        return try handler(request)
    }

    var requests: [URLRequest] {
        lock.withLock { seen }
    }
}

final class StubURLProtocol: URLProtocol {
    override static func canInit(with request: URLRequest) -> Bool { true }
    override static func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        do {
            let (response, data) = try StubResponder.shared.respond(to: request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}

enum StubTransport {
    static func session() -> URLSession {
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [StubURLProtocol.self]
        return URLSession(configuration: config)
    }

    static func client(
        baseURL: String = "https://planty.test",
        token: String? = "s3cret"
    ) -> PlantyClient {
        PlantyClient(
            configuration: PlantyConfiguration(baseURL: URL(string: baseURL), token: token),
            session: session()
        )
    }

    static func respond(status: Int = 200, json: String) {
        StubResponder.shared.install { request in
            let response = HTTPURLResponse(
                url: request.url ?? URL(fileURLWithPath: "/"),
                statusCode: status,
                httpVersion: "HTTP/1.1",
                headerFields: ["Content-Type": "application/json"]
            )
            guard let response else { throw URLError(.badServerResponse) }
            return (response, Data(json.utf8))
        }
    }

    static func fail(with error: URLError.Code) {
        StubResponder.shared.install { _ in throw URLError(error) }
    }
}
