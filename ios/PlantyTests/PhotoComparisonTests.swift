import Foundation
import Testing

@testable import Planty

@Suite("Photo comparison")
struct PhotoComparisonTests {
    private let epoch = Date(timeIntervalSince1970: 1_700_000_000)

    private func photo(daysAgo: Int) -> Photo {
        Photo(
            id: UUID(),
            plantID: UUID(),
            storageKey: "k\(daysAgo)",
            takenAt: epoch.addingTimeInterval(-Double(daysAgo) * 86_400),
            createdAt: epoch
        )
    }

    @Test("Photos are put oldest first however they arrived")
    func ordersOldestFirst() {
        let comparison = PhotoComparison([photo(daysAgo: 1), photo(daysAgo: 90), photo(daysAgo: 30)])

        #expect(comparison.earliest?.storageKey == "k90")
        #expect(comparison.latest?.storageKey == "k1")
    }

    /// Offering a comparison of one photo against itself invites the reading
    /// that nothing has changed.
    @Test("One photo is not a comparison")
    func needsTwoPhotos() {
        #expect(!PhotoComparison([]).isPossible)
        #expect(!PhotoComparison([photo(daysAgo: 1)]).isPossible)
        #expect(PhotoComparison([photo(daysAgo: 1), photo(daysAgo: 2)]).isPossible)
    }

    /// A slider bound to a count that just shrank reads off the end mid
    /// gesture, so the index clamps rather than trapping.
    @Test("An index past either end clamps instead of crashing")
    func clampsIndex() {
        let comparison = PhotoComparison([photo(daysAgo: 30), photo(daysAgo: 1)])

        #expect(comparison.photo(at: -5)?.storageKey == "k30")
        #expect(comparison.photo(at: 99)?.storageKey == "k1")
        #expect(PhotoComparison([]).photo(at: 0) == nil)
    }

    @Test("The gap between two photos is said in whole units")
    func describesTheSpan() {
        let now = epoch
        #expect(PhotoComparison.span(between: now, and: now) == "the same day")
        #expect(PhotoComparison.span(between: now, and: now.addingTimeInterval(3 * 86_400)).contains("3"))
        #expect(PhotoComparison.span(between: now, and: now.addingTimeInterval(3 * 86_400)).hasSuffix("apart"))
    }

    /// Which photo came first is the question, not which argument came first.
    @Test("The gap reads the same in either direction")
    func spanIsSymmetric() {
        let later = epoch.addingTimeInterval(40 * 86_400)

        #expect(
            PhotoComparison.span(between: epoch, and: later)
                == PhotoComparison.span(between: later, and: epoch)
        )
    }
}
