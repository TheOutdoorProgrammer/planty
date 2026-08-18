import Foundation
import Testing

@testable import Planty

/// The cold warning arrives on a phone, and until this the only way to answer
/// it was to talk to an agent or run curl.
@Suite("Shelter")
struct ShelterTests {
    @Test("A plant reads the shelter state the service has always sent")
    func decodesShelteredAt() throws {
        let json = """
            {
              "id": "\(UUID().uuidString)",
              "slug": "peace-lily",
              "common_name": "Peace Lily",
              "domain": "houseplant",
              "steward": "Marcus",
              "status": "alive",
              "location": "front porch",
              "accessibility": "easy",
              "watering_method": "hand",
              "care_profile": {},
              "created_at": "2026-08-01T09:00:00Z",
              "updated_at": "2026-08-01T09:00:00Z",
              "sheltered_at": "2026-08-14T22:00:00Z"
            }
            """
        let decoded = try PlantyCoders.decoder().decode(Plant.self, from: Data(json.utf8))

        #expect(decoded.isSheltered)
    }

    @Test("A plant with no shelter stamp is outside")
    func absentMeansOutside() {
        #expect(!Plant.fixture().isSheltered)
    }

    /// Offering to bring in a plant that was never asked to come in is noise,
    /// and offering to put out one with no threshold is meaningless.
    @Test("Only a plant with a cold threshold can be moved for cold")
    func onlyThresholdPlantsOffer() {
        var tender = Plant.fixture()
        tender.minTempF = 55
        #expect(tender.canShelter)

        var hardy = Plant.fixture()
        hardy.minTempF = nil
        #expect(!hardy.canShelter)

        var gone = Plant.fixture()
        gone.minTempF = 55
        gone.archivedAt = Date()
        #expect(!gone.canShelter)
    }

    @Test("Sheltering names the plant and moves the local state straight away")
    @MainActor
    func recordsAndReflects() async {
        var tender = Plant.fixture()
        tender.minTempF = 55
        let api = FakeAPI()
        let store = PlantStoryStore(api: api, plant: tender)

        await store.setSheltered(true)
        #expect(store.plant.isSheltered)

        await store.setSheltered(false)
        #expect(!store.plant.isSheltered)
    }

    /// A failed write must not leave the button claiming the plant is inside
    /// when the service never heard about it.
    @Test("A refused write leaves the state alone and surfaces the error")
    @MainActor
    func aFailedWriteChangesNothing() async {
        var tender = Plant.fixture()
        tender.minTempF = 55
        let api = FakeAPI()
        api.failure = PlantyError.offline
        let store = PlantStoryStore(api: api, plant: tender)

        await store.setSheltered(true)

        #expect(!store.plant.isSheltered)
        #expect(store.error != nil)
    }
}
