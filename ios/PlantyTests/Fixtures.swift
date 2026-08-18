import Foundation

@testable import Planty

extension Date {
    /// A fixed clock, so "3 days ago" means the same thing on every run.
    static let reference = Date(timeIntervalSince1970: 1_755_500_000)

    func minus(hours: Double) -> Date {
        addingTimeInterval(-hours * 3600)
    }

    func minus(days: Double) -> Date {
        minus(hours: days * 24)
    }
}

extension Plant {
    static func fixture(
        slug: String = "mona",
        commonName: String = "Mona",
        steward: String = Plant.stewardSelf,
        status: PlantStatus = .alive,
        location: String = "Living room",
        accessibility: PlantAccessibility = .easy,
        wateringMethod: WateringMethod = .hand,
        id: UUID = UUID()
    ) -> Plant {
        Plant(
            id: id,
            slug: slug,
            commonName: commonName,
            domain: .houseplant,
            steward: steward,
            status: status,
            location: location,
            accessibility: accessibility,
            wateringMethod: wateringMethod,
            careProfile: CareProfile(),
            createdAt: .reference,
            updatedAt: .reference
        )
    }
}

extension Verdict {
    static func fixture(
        action: VerdictAction = .water,
        forDate: Date = .reference,
        plantID: UUID = UUID(),
        reasoning: String = "The soil has stayed dry for two days.",
        confidence: Double = 0.8,
        acknowledgedAt: Date? = nil
    ) -> Verdict {
        Verdict(
            id: UUID(),
            plantID: plantID,
            forDate: forDate,
            action: action,
            reasoning: reasoning,
            confidence: confidence,
            evidence: Evidence(),
            createdAt: forDate,
            acknowledgedAt: acknowledgedAt
        )
    }
}

extension DigestEntry {
    static func fixture(
        plant: Plant = .fixture(),
        action: VerdictAction = .water,
        forDate: Date = .reference
    ) -> DigestEntry {
        DigestEntry(
            plant: plant,
            verdict: .fixture(action: action, forDate: forDate, plantID: plant.id),
            risk: plant.risk
        )
    }
}

extension Digest {
    static func fixture(
        date: Date = .reference,
        entries: [DigestEntry] = [],
        checked: Int = 8,
        staleSince: Date? = nil
    ) -> Digest {
        Digest(date: date, entries: entries, checked: checked, staleSince: staleSince)
    }
}

extension TodayInputs {
    static func fixture(
        isConfigured: Bool = true,
        isLoading: Bool = false,
        digest: Digest? = .fixture(),
        error: PlantyError? = nil,
        knownPlantCount: Int? = 8,
        now: Date = .reference
    ) -> TodayInputs {
        TodayInputs(
            isConfigured: isConfigured,
            isLoading: isLoading,
            digest: digest,
            error: error,
            knownPlantCount: knownPlantCount,
            now: now
        )
    }
}
