import Foundation
import Testing

@testable import Planty

@Suite("RFC3339 timestamps Go actually emits")
struct DateDecodingTests {
    @Test("No fractional seconds, which is what Go writes for a whole second")
    func wholeSeconds() throws {
        let date = try #require(PlantyDateFormat.date(from: "2026-08-18T08:04:32Z"))
        #expect(date.timeIntervalSince1970 == 1_755_504_272)
    }

    @Test("Three fractional digits")
    func milliseconds() throws {
        let date = try #require(PlantyDateFormat.date(from: "2026-08-18T08:04:32.250Z"))
        #expect(abs(date.timeIntervalSince1970 - 1_755_504_272.25) < 0.001)
    }

    @Test("Nine fractional digits, which ISO8601DateFormatter rejects untouched")
    func nanoseconds() throws {
        let date = try #require(PlantyDateFormat.date(from: "2026-08-18T08:04:32.123456789Z"))
        #expect(abs(date.timeIntervalSince1970 - 1_755_504_272.123) < 0.002)
    }

    @Test("A numeric offset instead of Z")
    func offset() throws {
        let withOffset = try #require(PlantyDateFormat.date(from: "2026-08-18T04:04:32-04:00"))
        let asZulu = try #require(PlantyDateFormat.date(from: "2026-08-18T08:04:32Z"))
        #expect(withOffset == asZulu)
    }

    @Test("An offset survives fraction clipping")
    func offsetWithNanoseconds() throws {
        let clipped = PlantyDateFormat.clippingFraction("2026-08-18T04:04:32.987654321-04:00")
        #expect(clipped == "2026-08-18T04:04:32.987-04:00")
        #expect(PlantyDateFormat.date(from: "2026-08-18T04:04:32.987654321-04:00") != nil)
    }

    @Test("A string with no fraction is returned untouched")
    func clippingIsANoOpWithoutAFraction() {
        let raw = "2026-08-18T08:04:32Z"
        #expect(PlantyDateFormat.clippingFraction(raw) == raw)
    }

    @Test("Garbage fails loudly rather than decoding to a wrong instant")
    func garbageFails() {
        #expect(PlantyDateFormat.date(from: "yesterday-ish") == nil)
    }

    @Test("The decoder surfaces a bad timestamp as a decoding error")
    func decoderThrows() {
        let json = Data(#"{"taken_at":"nope"}"#.utf8)
        #expect(throws: (any Error).self) {
            try PlantyCoders.decoder().decode(TakenAtProbe.self, from: json)
        }
    }

    @Test("Encoding round-trips through the decoder")
    func roundTrip() throws {
        let original = Date(timeIntervalSince1970: 1_755_504_272.5)
        let encoded = PlantyDateFormat.string(from: original)
        let decoded = try #require(PlantyDateFormat.date(from: encoded))
        #expect(abs(decoded.timeIntervalSince1970 - original.timeIntervalSince1970) < 0.002)
    }
}

private struct TakenAtProbe: Decodable {
    let takenAt: Date

    enum CodingKeys: String, CodingKey {
        case takenAt = "taken_at"
    }
}
