import Foundation
import Testing

@testable import Planty

@Suite("What Today decides to show")
struct TodayPresentationTests {
    @Test("No base URL means say so, not show a calm screen")
    func unconfigured() {
        let presentation = TodayPresentation.make(.fixture(isConfigured: false))
        #expect(presentation == .unconfigured)
    }

    @Test("A first load with nothing cached")
    func coldLoad() {
        let presentation = TodayPresentation.make(.fixture(isLoading: true, digest: nil))
        #expect(presentation == .loadingCold)
    }

    @Test("A refresh keeps the previous answer on screen")
    func warmLoad() {
        let digest = Digest.fixture(date: Date.reference.minus(hours: 24))
        let presentation = TodayPresentation.make(.fixture(isLoading: true, digest: digest))
        #expect(presentation == .loadingWarm(previous: digest, checkedAt: digest.date))
    }

    @Test("Nothing to do, on fresh evidence, is the calm state")
    func calm() throws {
        let digest = Digest.fixture(date: Date.reference.minus(hours: 1), checked: 8)
        guard case .calm(let summary) = TodayPresentation.make(.fixture(digest: digest)) else {
            Issue.record("expected the calm state")
            return
        }
        #expect(summary.checked == 8)
        #expect(summary.headline == "You're done.")
        #expect(summary.freshnessLine.contains("8 plants checked"))
    }

    @Test("The same empty digest, gone stale, is NOT calm")
    func staleIsNotCalm() {
        let digest = Digest.fixture(date: Date.reference.minus(days: 3), checked: 8)
        let presentation = TodayPresentation.make(.fixture(digest: digest))
        guard case .stale(let summary) = presentation else {
            Issue.record("a three day old digest must never present as calm")
            return
        }
        #expect(summary.headline == "Planty needs a fresh look.")
        #expect(summary.body.contains("3 days"))
        #expect(presentation.showsMascot == false)
    }

    @Test("A service-reported stale digest is stale even when minutes old")
    func serviceReportedStale() {
        let digest = Digest.fixture(
            date: Date.reference.minus(hours: 1),
            staleSince: Date.reference.minus(days: 2)
        )
        guard case .stale = TodayPresentation.make(.fixture(digest: digest)) else {
            Issue.record("stale_since must win over a recent digest date")
            return
        }
    }

    @Test("Stale still shows the actions it already knew about")
    func stalePreservesPendingWork() {
        let digest = Digest.fixture(
            date: Date.reference.minus(days: 3),
            entries: [.fixture(action: .urgent)]
        )
        guard case .stale(let summary) = TodayPresentation.make(.fixture(digest: digest)) else {
            Issue.record("expected the stale state")
            return
        }
        #expect(summary.pending.count == 1)
        #expect(summary.pendingLabel != nil)
    }

    @Test("An empty library is an onboarding path, not a calm claim")
    func emptySetup() {
        let digest = Digest.fixture(checked: 0)
        let presentation = TodayPresentation.make(
            .fixture(digest: digest, knownPlantCount: 0)
        )
        #expect(presentation == .emptySetup)
    }

    @Test("A failed load never falls through to reassurance")
    func failureIsNotCalm() {
        let presentation = TodayPresentation.make(
            .fixture(digest: nil, error: .timedOut, knownPlantCount: nil)
        )
        #expect(presentation == .failed(error: .timedOut, cached: nil))
        #expect(presentation.showsMascot == false)
    }

    @Test("A failed refresh keeps the cached digest to show underneath")
    func failureKeepsCache() {
        let digest = Digest.fixture(entries: [.fixture()])
        let presentation = TodayPresentation.make(.fixture(digest: digest, error: .offline))
        #expect(presentation == .failed(error: .offline, cached: digest))
    }

    @Test("One action reads as one thing")
    func singleAction() {
        let digest = Digest.fixture(
            date: Date.reference.minus(hours: 1),
            entries: [.fixture()]
        )
        guard case .actions(let summary) = TodayPresentation.make(.fixture(digest: digest)) else {
            Issue.record("expected the action state")
            return
        }
        #expect(summary.headline == "One thing. You've got this.")
        #expect(summary.featured.count == 1)
        #expect(summary.deferredLabel == nil)
        #expect(summary.footnote == "Everything else is okay.")
    }

    @Test("Never more than three expanded cards")
    func overflowCollapses() {
        let entries = (0..<5).map { index in
            DigestEntry.fixture(plant: .fixture(slug: "p\(index)", commonName: "Plant \(index)"))
        }
        let digest = Digest.fixture(date: Date.reference.minus(hours: 1), entries: entries)
        guard case .actions(let summary) = TodayPresentation.make(.fixture(digest: digest)) else {
            Issue.record("expected the action state")
            return
        }
        #expect(summary.featured.count == 3)
        #expect(summary.deferred.count == 2)
        #expect(summary.deferredLabel == "2 more for later today")
    }

    @Test("Friend-owned plants sort first, and get no red for it")
    func friendPlantsSortFirst() {
        let mine = DigestEntry.fixture(
            plant: .fixture(slug: "fern", commonName: "Fernie", steward: "self"),
            action: .urgent
        )
        let theirs = DigestEntry.fixture(
            plant: .fixture(slug: "mona", commonName: "Mona", steward: "Maya"),
            action: .water
        )
        let digest = Digest.fixture(
            date: Date.reference.minus(hours: 1),
            entries: [mine, theirs]
        )

        #expect(digest.sortedEntries.first?.plant.steward == "Maya")
        #expect(CareState.from(action: theirs.verdict.action).color != PlantyColor.red)
    }

    @Test("Among equals, the service's own neglect risk breaks the tie")
    func riskOrdersWithinAGroup() {
        let easy = DigestEntry.fixture(
            plant: .fixture(slug: "a", commonName: "A", accessibility: .easy, wateringMethod: .letpot),
            risk: 1
        )
        let hard = DigestEntry.fixture(
            plant: .fixture(slug: "b", commonName: "B", accessibility: .hard, wateringMethod: .hand),
            risk: 7
        )
        let digest = Digest.fixture(entries: [easy, hard])
        #expect(digest.sortedEntries.first?.plant.slug == "b")
    }

    @Test("The mascot appears in calm and never beside an alert")
    func mascotBlastRadius() {
        let calm = TodayPresentation.calm(CalmSummary(checked: 8, updatedAt: .reference))
        #expect(calm.showsMascot)

        let actions = TodayPresentation.actions(
            ActionSummary(featured: [.fixture()], deferred: [], checked: 8, updatedAt: .reference)
        )
        #expect(!actions.showsMascot)

        let stale = TodayPresentation.stale(
            StaleSummary(since: .reference, reason: .tooOld, now: .reference, pending: [])
        )
        #expect(!stale.showsMascot)
    }
}
