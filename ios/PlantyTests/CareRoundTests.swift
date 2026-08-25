import Testing

@testable import Planty

@Suite("Room-grouped care rounds")
struct CareRoundTests {
    @Test("Urgent rooms come first and tasks stay grouped")
    func urgentRoomsFirst() {
        var office = Plant.fixture(slug: "office")
        office.location = "Desk"
        office.haArea = "Office"
        var kitchen = Plant.fixture(slug: "kitchen")
        kitchen.location = "Kitchen"
        let digest = Digest(
            date: .reference,
            entries: [
                .fixture(plant: office, action: .water),
                .fixture(plant: kitchen, action: .urgent)
            ],
            checked: 2
        )

        let groups = CareRoundPlanner.groups(digest: digest)

        #expect(groups.map(\.room) == ["Kitchen", "Office"])
        #expect(groups.map(\.count) == [1, 1])
    }

    @Test("Resolved work leaves the round")
    func resolvedWorkLeaves() {
        let entry = DigestEntry.fixture(action: .check)
        let digest = Digest(date: .reference, entries: [entry], checked: 1)

        let groups = CareRoundPlanner.groups(
            digest: digest,
            resolvedVerdicts: [entry.verdict.id]
        )

        #expect(groups.isEmpty)
    }
}
