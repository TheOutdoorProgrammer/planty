import Foundation
import Testing

@testable import Planty

@Suite("Managed household choices", .serialized)
struct ManagedChoicesTests {
    @Test("The client loads the server catalog from one route")
    func clientLoadsCatalog() async throws {
        StubTransport.respond(json: """
            {
              "places": {
                "recent": [{"value":"Living Room","sources":["plant_location"],"last_used_at":"2026-08-20T01:00:00Z"}],
                "all": [{"value":"Living Room","sources":["home_assistant","plant_location"],"last_used_at":"2026-08-20T01:00:00Z"}]
              },
              "owners": {
                "recent": [],
                "all": [{"value":"self","sources":["default"]}]
              },
              "pot_materials": {"recent": [], "all": []}
            }
            """)

        let catalog = try await StubTransport.client().managedChoices()
        let request = try #require(StubResponder.shared.requests.first)

        #expect(request.httpMethod == "GET")
        #expect(request.url?.path == "/v1/choices")
        #expect(catalog.places.recent.map(\.value) == ["Living Room"])
        #expect(catalog.places.all.first?.sources.contains("home_assistant") == true)
        #expect(catalog.owners.all.map(\.value) == ["self"])
    }

    @Test("Changing a legacy plant's Place converges room and Home Assistant area")
    func placeEditConvergesLegacyFields() throws {
        var plant = Plant.fixture(location: "Window shelf")
        plant.haArea = "Living Room"
        var form = PlantEditForm(plant: plant)

        #expect(try form.patch(against: plant).get().isEmpty)

        form.location = "Back Porch"
        let patch = try form.patch(against: plant).get()
        #expect(patch.location == "Back Porch")
        #expect(patch.haArea == "Back Porch")
    }

    @Test("New plants encode one selected Place into both legacy fields")
    func newPlantCarriesSharedPlace() throws {
        var plant = NewPlant(commonName: "Fern")
        plant.location = "Living Room"
        plant.haArea = "Living Room"
        plant.steward = "Maya"
        plant.potMaterial = "Terracotta"

        let data = try PlantyCoders.encoder().encode(plant)
        let body = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
        #expect(body["location"] as? String == "Living Room")
        #expect(body["ha_area"] as? String == "Living Room")
        #expect(body["steward"] as? String == "Maya")
        #expect(body["pot_material"] as? String == "Terracotta")
    }
}
