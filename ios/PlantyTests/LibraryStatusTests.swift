import Foundation
import Testing

@testable import Planty

@Suite("The status word on a library row")
struct LibraryStatusTests {
    private let mona = Plant.fixture(slug: "mona", commonName: "Mona")

    @Test("With no digest at all, a plant is Unknown, not All good")
    func noDigest() {
        let state = LibraryStatus.state(
            for: mona,
            digest: nil,
            now: .reference,
            knownPlantCount: 3
        )
        #expect(state == .unknown)
    }

    @Test("A fresh run that did not flag this plant means All good")
    func freshAndUnflagged() {
        let digest = Digest.fixture(date: Date.reference.minus(hours: 1), checked: 3)
        let state = LibraryStatus.state(
            for: mona,
            digest: digest,
            now: .reference,
            knownPlantCount: 3
        )
        #expect(state == .allGood)
    }

    @Test("A stale run that did not flag this plant means Unknown")
    func staleAndUnflagged() {
        let digest = Digest.fixture(date: Date.reference.minus(days: 3), checked: 3)
        let state = LibraryStatus.state(
            for: mona,
            digest: digest,
            now: .reference,
            knownPlantCount: 3
        )
        #expect(state == .unknown)
    }

    @Test("A flagged plant keeps its action even from a stale run")
    func staleButFlagged() {
        let entry = DigestEntry.fixture(plant: mona, action: .urgent)
        let digest = Digest.fixture(date: Date.reference.minus(days: 3), entries: [entry])
        let state = LibraryStatus.state(
            for: mona,
            digest: digest,
            now: .reference,
            knownPlantCount: 3
        )
        #expect(state == .urgent)
    }

    @Test("A fresh flagged plant reads as its action")
    func freshAndFlagged() {
        let entry = DigestEntry.fixture(plant: mona, action: .water)
        let digest = Digest.fixture(date: Date.reference.minus(hours: 1), entries: [entry])
        let state = LibraryStatus.state(
            for: mona,
            digest: digest,
            now: .reference,
            knownPlantCount: 3
        )
        #expect(state == .needsCare)
    }
}
