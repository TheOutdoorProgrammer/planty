import Foundation

@testable import Planty

private let stubResponderHeader = "X-Planty-Test-Responder"

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

/// URLProtocol classes are process-wide, so isolated sessions identify their
/// responder with a private request header and resolve it through this locked
/// registry. Tests that still use the legacy static StubTransport fall back to
/// StubResponder.shared.
private final class StubResponderRegistry: @unchecked Sendable {
    static let shared = StubResponderRegistry()

    private let lock = NSLock()
    private var responders: [String: StubResponder] = [:]

    func register(_ responder: StubResponder, id: String) {
        lock.withLock { responders[id] = responder }
    }

    func unregister(id: String) {
        lock.withLock { responders.removeValue(forKey: id) }
    }

    func responder(for request: URLRequest) -> StubResponder? {
        guard let id = request.value(forHTTPHeaderField: stubResponderHeader) else { return nil }
        return lock.withLock { responders[id] }
    }
}

final class StubURLProtocol: URLProtocol {
    override static func canInit(with request: URLRequest) -> Bool { true }
    override static func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        do {
            let responder = StubResponderRegistry.shared.responder(for: request) ?? StubResponder.shared
            let (response, data) = try responder.respond(to: request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}

/// A per-test transport for suites that can execute concurrently with other
/// network suites. Its handler and captured requests cannot be overwritten by
/// another test's StubTransport.respond call.
final class IsolatedStubTransport {
    private let id = UUID().uuidString
    private let responder = StubResponder()

    init() {
        StubResponderRegistry.shared.register(responder, id: id)
    }

    deinit {
        StubResponderRegistry.shared.unregister(id: id)
    }

    var requests: [URLRequest] { responder.requests }

    func client(
        baseURL: String = "https://planty.test",
        token: String? = "s3cret"
    ) -> PlantyClient {
        PlantyClient(
            configuration: PlantyConfiguration(baseURL: URL(string: baseURL), token: token),
            session: session()
        )
    }

    func respond(status: Int = 200, json: String) {
        responder.install { request in
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

    func fail(with error: URLError.Code) {
        responder.install { _ in throw URLError(error) }
    }

    private func session() -> URLSession {
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [StubURLProtocol.self]
        config.httpAdditionalHeaders = [stubResponderHeader: id]
        return URLSession(configuration: config)
    }
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

extension URLRequest {
    /// URLProtocol is handed the body as a stream and leaves `httpBody` nil, so
    /// asserting on what actually went out means draining it.
    var stubbedBody: Data? {
        if let httpBody { return httpBody }
        guard let stream = httpBodyStream else { return nil }
        stream.open()
        defer { stream.close() }

        var body = Data()
        var buffer = [UInt8](repeating: 0, count: 4096)
        while stream.hasBytesAvailable {
            let read = stream.read(&buffer, maxLength: buffer.count)
            guard read > 0 else { break }
            body.append(buffer, count: read)
        }
        return body
    }
}
