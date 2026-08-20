import Foundation
import Testing

@testable import Planty

@Suite("Away-period management")
struct AwayPeriodManagementTests {
    @Test("Coverage is half-open at the return time")
    func coverageWindow() {
        let start = Date(timeIntervalSince1970: 4_200_000_000)
        let end = start.addingTimeInterval(3_600)
        let period = AwayPeriod(
            id: UUID(), startsAt: start, endsAt: end,
            createdAt: .reference
        )

        #expect(period.covers(start))
        #expect(period.covers(end.addingTimeInterval(-1)))
        #expect(!period.covers(end))
    }

    @Test("An edit encodes blank optional metadata as explicit empty strings")
    func clearingMetadata() throws {
        let start = Date(timeIntervalSince1970: 4_200_000_000)
        let update = AwayPeriodUpdate(
            NewAwayPeriod(startsAt: start, endsAt: start.addingTimeInterval(3_600))
        )
        let data = try JSONEncoder().encode(update)
        let body = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])

        #expect(body["backup_contact"] as? String == "")
        #expect(body["backup_notify"] as? String == "")
        #expect(body["note"] as? String == "")
    }
}
