import Foundation
import Testing

@testable import Planty

/// The draft is what keeps Link disabled until the service would say yes, so
/// its refusals matter as much as what it builds.
@Suite("Sensor link draft")
struct SettingsSensorLinkTests {
    @Test("Only domain-dot-name reads as an entity id")
    func recognisesEntityIDShape() {
        var draft = SensorLinkDraft()

        draft.haEntityID = "sensor.monstera_soil_moisture"
        #expect(draft.entityIDLooksRight)

        draft.haEntityID = "  sensor.monstera_soil_moisture  "
        #expect(draft.entityIDLooksRight)

        for wrong in ["", "Monstera moisture", "sensor.", "sensor.pot 7", "a.b.c", "monstera"] {
            draft.haEntityID = wrong
            #expect(!draft.entityIDLooksRight, "\(wrong) should be refused")
        }
    }

    @Test("A plant link needs a plant")
    func plantTargetNeedsAPlant() {
        var draft = SensorLinkDraft()
        draft.haEntityID = "sensor.pot_7"
        draft.target = .plant
        #expect(draft.newLink() == nil)

        let plantID = UUID()
        draft.plantID = plantID
        let link = draft.newLink()
        #expect(link?.plantID == plantID)
        #expect(link?.zone == nil)
        #expect(link?.haEntityID == "sensor.pot_7")
    }

    @Test("A zone link needs a zone, and gets it trimmed")
    func zoneTargetNeedsAZone() {
        var draft = SensorLinkDraft()
        draft.haEntityID = "sensor.porch_temp"
        draft.role = .ambientTemp
        draft.target = .zone
        #expect(draft.newLink() == nil)

        draft.zone = "  porch  "
        let link = draft.newLink()
        #expect(link?.zone == "porch")
        #expect(link?.plantID == nil)
        #expect(link?.role == .ambientTemp)
    }

    @Test("Soil moisture cannot speak for a whole place")
    func soilMoistureNeedsOnePlant() {
        var draft = SensorLinkDraft()
        draft.haEntityID = "sensor.greenhouse_soil"
        draft.role = .soilMoisture
        draft.target = .zone
        draft.zone = "Greenhouse"

        #expect(draft.newLink() == nil)
    }

    /// A chosen plant must not ride along on a zone link: the service takes
    /// exactly one of the two.
    @Test("Switching target drops the other half")
    func targetsAreExclusive() {
        var draft = SensorLinkDraft()
        draft.haEntityID = "sensor.porch_temp"
        draft.role = .ambientTemp
        draft.plantID = UUID()
        draft.target = .zone
        draft.zone = "porch"

        let link = draft.newLink()
        #expect(link?.plantID == nil)
        #expect(link?.zone == "porch")
    }

    @Test("A malformed id blocks the link outright")
    func malformedIDBlocksEverything() {
        var draft = SensorLinkDraft()
        draft.haEntityID = "not an id"
        draft.target = .zone
        draft.zone = "porch"
        #expect(draft.newLink() == nil)
    }

    @Test("The unknown role is never offered")
    func neverOffersUnknown() {
        #expect(!SensorLinkDraft.offerableRoles.contains(.unknown))
        #expect(!SensorLinkDraft.offerableRoles.isEmpty)
    }

    @Test("Every offered sensor role can move to another plant")
    func everyRoleCanMoveToAnotherPlant() throws {
        let first = UUID()
        let second = UUID()
        for role in SensorLinkDraft.offerableRoles {
            let link = SensorLink(
                id: UUID(),
                plantID: first,
                zone: nil,
                haEntityID: "sensor.test",
                role: role,
                createdAt: .reference
            )
            var draft = SensorAssignmentDraft(link: link)
            draft.plantID = second

            let assignment = try #require(draft.assignment(for: role))
            #expect(assignment.plantID == second)
            #expect(assignment.zone == nil)
        }
    }

    @Test("Ambient sensors can move between a plant and a place")
    func ambientSensorsCanMoveToAPlace() throws {
        let link = SensorLink(
            id: UUID(),
            plantID: UUID(),
            zone: nil,
            haEntityID: "sensor.room_temperature",
            role: .ambientTemp,
            createdAt: .reference
        )
        var draft = SensorAssignmentDraft(link: link)
        draft.target = .zone
        draft.zone = "  Living room  "

        let assignment = try #require(draft.assignment(for: .ambientTemp))
        #expect(assignment.plantID == nil)
        #expect(assignment.zone == "Living room")
        #expect(draft.assignment(for: .soilMoisture) == nil)
    }
}
