import Foundation

@testable import Planty

extension Date {
    /// 2026-08-18T08:04:32Z, the moment the design docs are written around.
    static let reference = Date(timeIntervalSince1970: 1_787_040_272)

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
        now: Date = .reference,
        didJustFinish: Bool = false
    ) -> TodayInputs {
        TodayInputs(
            isConfigured: isConfigured,
            isLoading: isLoading,
            digest: digest,
            error: error,
            knownPlantCount: knownPlantCount,
            now: now,
            didJustFinish: didJustFinish
        )
    }
}

extension PlantAnswer {
    static func fixture(
        reply: String = "It does not need water today.",
        confidence: Double = 0.9,
        lookedAt: String? = nil,
        followUps: [String] = [],
        conversationID: UUID = UUID()
    ) -> PlantAnswer {
        PlantAnswer(
            id: UUID(),
            conversationID: conversationID,
            reply: reply,
            confidence: confidence,
            lookedAt: lookedAt,
            suggestedFollowUps: followUps
        )
    }
}

extension Toxicity {
    /// The lily: the case the whole feature exists for, where the three
    /// audiences genuinely disagree.
    static func lily() -> Toxicity {
        Toxicity(
            cats: .severe,
            dogs: .mild,
            people: .mild,
            basis: .source,
            identifiedAs: "Lilium longiflorum",
            principle: "unidentified nephrotoxin",
            signs: "vomiting, then anuria",
            parts: ["all", "flower"],
            routes: ["eaten", "skin"],
            notes: "why the audiences differ",
            firstAid: "a cat that groomed pollen goes to a vet before any sign appears",
            source: "www.aspca.org",
            checkedAt: .reference
        )
    }

    /// Every aroid Planty will ever see: identical for all three, which is why
    /// divergence is worth shouting about when it happens.
    static func pothos() -> Toxicity {
        Toxicity(cats: .mild, dogs: .mild, people: .mild, basis: .source, checkedAt: .reference)
    }
}

extension Reminder {
    static func fixture(
        kind: ObservationKind = .misted,
        everyDays: Int = 1,
        atHours: [Int] = [8, 20],
        active: Bool = true,
        lastDone: Date? = nil,
        due: Bool? = nil
    ) -> Reminder {
        Reminder(
            id: UUID(),
            plantID: UUID(),
            kind: kind,
            everyDays: everyDays,
            atHours: atHours,
            active: active,
            note: nil,
            lastDone: lastDone,
            due: due
        )
    }
}

extension PlantNote {
    static func fixture(
        id: UUID = UUID(),
        plantID: UUID = UUID(),
        title: String? = nil,
        body: String = "the cat keeps chewing this one",
        createdAt: Date = Date(timeIntervalSince1970: 1_760_000_000),
        updatedAt: Date? = nil
    ) -> PlantNote {
        PlantNote(
            id: id,
            plantID: plantID,
            title: title,
            body: body,
            createdAt: createdAt,
            updatedAt: updatedAt ?? createdAt
        )
    }
}
