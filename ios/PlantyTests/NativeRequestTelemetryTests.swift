import Foundation
import NativeTelemetry
import Testing
@testable import Planty

private actor NativeEventRecorder {
    var events: [NativeEvent] = []
    func record(_ event: NativeEvent) { events.append(event) }
}

struct NativeRequestTelemetryTests {
    @Test(arguments: [200, 503])
    func realRequestHeaderMatchesExportedCompletionEvent(status: Int) async throws {
        let stub = IsolatedStubTransport()
        stub.respond(status: status, json: #"{"error":"private-fixture"}"#)
        let recorder = NativeEventRecorder()
        let original = stub.client()
        let client = PlantyClient(configuration: original.configuration, session: original.session) {
            await recorder.record($0)
        }
        do {
            _ = try await client.perform(URLRequest(url: URL(string: "https://planty.test/v1/today")!))
            #expect(status == 200)
        } catch {
            #expect(status == 503)
        }
        let event = try #require(await recorder.events.first)
        let envelope = NativeEnvelope(release: NativeRelease(release: "1.1", build: "165")!, events: [event])
        let encoded = try NativeCoding.encoder().encode(envelope)
        let object = try #require(JSONSerialization.jsonObject(with: encoded) as? [String: Any])
        let events = try #require(object["events"] as? [[String: Any]])
        let exportedID = try #require(events.first?["id"] as? String)
        let traceID = exportedID.replacingOccurrences(of: "-", with: "").lowercased()
        #expect(stub.requests.first?.value(forHTTPHeaderField: "traceparent")
            == "00-\(traceID)-\(traceID.suffix(16))-01")
        #expect(event.outcome == (status == 200 ? .success : .failure))
        #expect(event.errorClass == (status == 200 ? .none : .server))
        #expect(try !#require(String(data: encoded, encoding: .utf8)).contains("private-fixture"))
    }

    @Test func callerContextSurvivesAndTimeoutStillReportsIndependentFailure() async throws {
        let stub = IsolatedStubTransport()
        stub.fail(with: .timedOut)
        let recorder = NativeEventRecorder()
        let original = stub.client()
        let client = PlantyClient(configuration: original.configuration, session: original.session) {
            await recorder.record($0)
        }
        var request = URLRequest(url: URL(string: "https://planty.test/v1/today")!)
        let existing = "00-11111111111141118111111111111111-8111111111111111-01"
        request.setValue(existing, forHTTPHeaderField: "traceparent")
        do {
            _ = try await client.perform(request)
            Issue.record("timeout unexpectedly succeeded")
        } catch {
            #expect(error as? PlantyError == .timedOut)
        }
        #expect(stub.requests.first?.value(forHTTPHeaderField: "traceparent") == existing)
        let event = try #require(await recorder.events.first)
        #expect(event.outcome == .failure)
        #expect(event.errorClass == .timeout)
        #expect(event.id.uuidString.replacingOccurrences(of: "-", with: "").lowercased()
            != "11111111111141118111111111111111")
    }
}
