import Foundation
import Testing

@testable import Planty

@Suite("Stale data is never allowed to read as calm")
struct FreshnessTests {
    @Test("A digest from this morning is fresh")
    func freshDigest() {
        let digest = Digest.fixture(date: Date.reference.minus(hours: 2))
        #expect(digest.freshness(now: .reference, knownPlantCount: 8) == .fresh)
    }

    @Test("The service saying it is stale outranks everything else")
    func serviceReportedWins() {
        let staleSince = Date.reference.minus(days: 3)
        let digest = Digest.fixture(
            date: Date.reference.minus(hours: 1),
            staleSince: staleSince
        )
        #expect(
            digest.freshness(now: .reference, knownPlantCount: 8)
                == .stale(since: staleSince, reason: .serviceReported)
        )
    }

    @Test("One missed morning is still explainable")
    func oneDayOldIsStillFresh() {
        let digest = Digest.fixture(date: Date.reference.minus(hours: 25))
        #expect(digest.freshness(now: .reference, knownPlantCount: 8) == .fresh)
    }

    @Test("Two missed mornings are not")
    func twoDaysOldIsStale() {
        let taken = Date.reference.minus(hours: 40)
        let digest = Digest.fixture(date: taken)
        #expect(
            digest.freshness(now: .reference, knownPlantCount: 8)
                == .stale(since: taken, reason: .tooOld)
        )
    }

    @Test("A run that read nothing, while plants exist, is stale not calm")
    func checkedNothing() {
        let digest = Digest.fixture(date: Date.reference.minus(hours: 1), checked: 0)
        #expect(
            digest.freshness(now: .reference, knownPlantCount: 8)
                == .stale(since: digest.date, reason: .checkedNothing)
        )
    }

    @Test("Checking nothing with no plants is simply an empty library")
    func checkedNothingWithNoPlants() {
        let digest = Digest.fixture(date: Date.reference.minus(hours: 1), checked: 0)
        #expect(digest.freshness(now: .reference, knownPlantCount: 0) == .fresh)
    }

    @Test("The tolerance is configurable and actually applied")
    func policyIsHonoured() {
        let digest = Digest.fixture(date: Date.reference.minus(hours: 5))
        let tight = FreshnessPolicy(maxAge: 3600)
        #expect(digest.freshness(now: .reference, knownPlantCount: 8, policy: tight).isFresh == false)
    }

    @Test("Relative wording says days, not a raw timestamp")
    func relativePhrase() {
        let phrase = RelativeAge.phrase(since: Date.reference.minus(days: 3), now: .reference)
        #expect(phrase.contains("3 days"))
    }

    @Test("Something that just happened does not claim to be hours old")
    func recentPhrase() {
        let phrase = RelativeAge.phrase(since: Date.reference.minus(hours: 0.2), now: .reference)
        #expect(phrase == "less than an hour ago")
    }

    @Test("Yesterday is named, not printed as a date")
    func yesterdayIsNamed() {
        let phrase = RelativeAge.dayAndTime(Date.reference.minus(hours: 24), now: .reference)
        #expect(phrase.hasPrefix("yesterday at"))
    }
}

@Suite("A plant's care state, once freshness is applied")
struct CareStateTests {
    @Test("No verdict at all means Unknown, never All good")
    func noVerdictIsUnknown() {
        #expect(CareState.resolve(verdict: nil, freshness: .fresh) == .unknown)
    }

    @Test("A fresh 'none' verdict is the only route to All good")
    func freshNoneIsAllGood() {
        let verdict = Verdict.fixture(action: .none)
        #expect(CareState.resolve(verdict: verdict, freshness: .fresh) == .allGood)
    }

    @Test("Stale evidence downgrades All good to Unknown")
    func staleNoneBecomesUnknown() {
        let verdict = Verdict.fixture(action: .none)
        let stale = Freshness.stale(since: Date.reference.minus(days: 3), reason: .tooOld)
        #expect(CareState.resolve(verdict: verdict, freshness: stale) == .unknown)
    }

    @Test("Stale evidence never hides an urgent verdict")
    func staleUrgentStaysUrgent() {
        let verdict = Verdict.fixture(action: .urgent)
        let stale = Freshness.stale(since: Date.reference.minus(days: 3), reason: .tooOld)
        #expect(CareState.resolve(verdict: verdict, freshness: stale) == .urgent)
    }

    @Test("Stale evidence never hides an action either")
    func staleWaterStaysNeedsCare() {
        let verdict = Verdict.fixture(action: .water)
        let stale = Freshness.stale(since: Date.reference.minus(days: 3), reason: .tooOld)
        #expect(CareState.resolve(verdict: verdict, freshness: stale) == .needsCare)
    }

    @Test("Only urgent is allowed to be red")
    func onlyUrgentIsRed() {
        for state in CareState.allCases where state != .urgent {
            #expect(state.color != PlantyColor.red)
        }
        #expect(CareState.urgent.color == PlantyColor.red)
    }

    @Test("No copy anywhere claims the plant is healthy")
    func neverClaimsHealth() {
        for state in CareState.allCases {
            #expect(!state.sentence.localizedCaseInsensitiveContains("healthy"))
        }
    }
}
