import Foundation
import Testing

@testable import Planty

/// Creating and correcting plants from the library side.
@Suite("The plant library's writes")
struct PlantsEditingTests {
    @Test("A failed create hands the error back and poisons nothing")
    @MainActor
    func createFailureIsReturnedNotSwallowed() async {
        let api = FakeAPI()
        api.failure = .offline
        let store = PlantsStore(api: api, isConfigured: true)

        let failure = await store.create(NewPlant(commonName: "Fern"))

        #expect(failure == .offline)
        #expect(store.plants.isEmpty)
        #expect(store.error == nil, "a create failure must not repaint the library as unloaded")
    }

    @Test("A successful create appends and reports nothing")
    @MainActor
    func createSuccessAppends() async {
        let store = PlantsStore(api: FakeAPI(), isConfigured: true)

        let failure = await store.create(NewPlant(commonName: "Fern"))

        #expect(failure == nil)
        #expect(store.plants.map(\.commonName) == ["Fern"])
    }

    @Test("A fresh copy replaces its row")
    @MainActor
    func applyReplacesTheMatchingPlant() async {
        let plant = Plant.fixture()
        let api = FakeAPI()
        api.plantList = [plant]
        let store = PlantsStore(api: api, isConfigured: true)
        await store.load()

        var updated = plant
        updated.commonName = "Renamed"
        store.apply(updated)

        #expect(store.plants.map(\.commonName) == ["Renamed"])
    }

    @Test("A retired plant leaves the list the way a reload would drop it")
    @MainActor
    func applyDropsRetiredPlants() async {
        let plant = Plant.fixture()
        let api = FakeAPI()
        api.plantList = [plant]
        let store = PlantsStore(api: api, isConfigured: true)
        await store.load()

        var updated = plant
        updated.status = .dead
        store.apply(updated)

        #expect(store.plants.isEmpty)
    }

    @Test("A copy without a photo cannot blank the one on screen")
    @MainActor
    func applyKeepsThePhotoThePatchAnswerLacks() async {
        var plant = Plant.fixture()
        plant.photoURL = URL(string: "https://planty.test/photos/mona.jpg")
        let api = FakeAPI()
        api.plantList = [plant]
        let store = PlantsStore(api: api, isConfigured: true)
        await store.load()

        var updated = plant
        updated.commonName = "Renamed"
        updated.photoURL = nil
        store.apply(updated)

        #expect(store.plants.first?.commonName == "Renamed")
        #expect(store.plants.first?.photoURL == plant.photoURL)
    }
}

/// The edit sheet's promise is "send only what changed"; this is that promise.
@Suite("Edit form diffing")
struct PlantsEditFormTests {
    private var plant: Plant {
        var plant = Plant.fixture()
        plant.botanicalName = "Monstera deliciosa"
        plant.minTempF = 40
        return plant
    }

    private func built(_ form: PlantEditForm, against plant: Plant) throws -> PlantPatch {
        try form.patch(against: plant).get()
    }

    @Test("An untouched form produces an empty patch")
    func untouchedFormIsEmpty() throws {
        let plant = plant
        let patch = try built(PlantEditForm(plant: plant), against: plant)

        #expect(patch.isEmpty)
    }

    @Test("Only the fields that changed go on the wire")
    func onlyChangesAreSent() throws {
        let plant = plant
        var form = PlantEditForm(plant: plant)
        form.commonName = "Monstera"
        form.minTempText = "45"

        var expected = PlantPatch()
        expected.commonName = "Monstera"
        expected.minTempF = 45

        #expect(try built(form, against: plant) == expected)
    }

    @Test("A number the keyboard mangled is refused, named field and all")
    func badNumbersAreRefused() {
        var form = PlantEditForm(plant: plant)
        form.minTempText = "40F"

        #expect(
            form.patch(against: plant)
                == .failure(.notANumber(field: "the cold limit", entered: "40F"))
        )
    }

    @Test("A comma decimal parses, because half the keyboards make one")
    func commaDecimalsParse() throws {
        let plant = plant
        var form = PlantEditForm(plant: plant)
        form.potSizeText = "6,5"

        #expect(try built(form, against: plant).potSizeIn == 6.5)
    }

    @Test("Emptying a botanical name clears it explicitly")
    func clearingAnOptionalSendsEmpty() throws {
        let plant = plant
        var form = PlantEditForm(plant: plant)
        form.botanicalName = ""

        #expect(try built(form, against: plant).botanicalName == "")
    }

    @Test("An emptied room reads as unchanged, not as erasure")
    func clearedLocationIsUnchanged() throws {
        let plant = plant
        var form = PlantEditForm(plant: plant)
        form.location = "   "

        #expect(try built(form, against: plant).isEmpty)
    }

    @Test("Recording drainage for the first time patches it")
    func drainageBecomesKnown() throws {
        let plant = plant
        var form = PlantEditForm(plant: plant)
        form.drainage = .drains

        #expect(try built(form, against: plant).hasDrainage == true)
    }
}
