import Foundation
import Testing

@testable import Planty

/// The gate is a cost control before it is a nicety: it is what stops a paid
/// species call on a photo of a dog or a receipt.
@Suite("Plant gate")
struct PlantGateTests {
    private func label(_ id: String, _ confidence: Double) -> CoarseLabel {
        CoarseLabel(identifier: id, confidence: confidence)
    }

    @Test("A plant label above the bar opens the gate")
    func passesOnPlant() {
        #expect(PlantGate.verdict(for: [label("houseplant", 0.82)]) == .plant)
        #expect(PlantGate.verdict(for: [label("plant_flower_rose", 0.55)]) == .plant)
    }

    @Test("A dog is not a plant")
    func rejectsAnimals() {
        let labels = [label("dog", 0.94), label("mammal", 0.88), label("pet", 0.7)]
        #expect(PlantGate.verdict(for: labels) == .notPlant)
    }

    @Test("A receipt is not a plant")
    func rejectsDocuments() {
        #expect(PlantGate.verdict(for: [label("document", 0.91), label("text", 0.77)]) == .notPlant)
    }

    /// Apple returns a long tail of near-zero labels. Counting those as
    /// evidence would open the gate on anything at all.
    @Test("A near-zero plant label is not evidence")
    func ignoresTheTail() {
        let labels = [label("dog", 0.96), label("plant", 0.02)]
        #expect(PlantGate.verdict(for: labels) == .notPlant)
        #expect(PlantGate.plantConfidence(in: labels) == 0)
    }

    /// A plant present but not dominant still deserves a species call: the
    /// gate's job is to exclude, not to be certain.
    @Test("A weak but real plant label still opens the gate")
    func passesOnModestConfidence() {
        #expect(PlantGate.verdict(for: [label("dog", 0.6), label("houseplant", 0.36)]) == .plant)
    }

    @Test("Matching is on whole segments, so plantation is not a plant")
    func matchesSegmentsNotSubstrings() {
        #expect(PlantGate.isPlantLike("houseplant"))
        #expect(PlantGate.isPlantLike("plant_flower_rose"))
        #expect(!PlantGate.isPlantLike("plantation"))
        #expect(!PlantGate.isPlantLike("eggplant_parmesan"))
    }

    @Test("What is shown is strongest first, capped, and free of the tail")
    func presentsTheTopFew() {
        let labels = [
            label("a", 0.9), label("b", 0.8), label("c", 0.7),
            label("d", 0.6), label("e", 0.5), label("f", 0.4),
            label("noise", 0.01)
        ]
        let shown = PlantGate.presentable(labels)

        #expect(shown.count == 5)
        #expect(shown.first?.identifier == "a")
        #expect(!shown.contains { $0.identifier == "noise" })
    }

    @Test("A dotted identifier reads as its leaf")
    func readsTheLeaf() {
        #expect(label("plant_flower_rose", 1).readable == "Rose")
        #expect(label("houseplant", 1).readable == "Houseplant")
    }
}
