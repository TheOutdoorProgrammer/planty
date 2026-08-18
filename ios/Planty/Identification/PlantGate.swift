import Foundation

/// Decides whether a photo is worth spending a species call on. A pure
/// function over labels, so it tests without Vision.
enum PlantGate {
    /// Apple's taxonomy is a dotted path, so matching a segment catches
    /// `plant_flower_rose` and `houseplant` without matching `plantation`.
    private static let plantWords: Set<String> = [
        "plant", "plants", "houseplant", "flower", "flowers", "leaf", "leaves",
        "tree", "trees", "shrub", "succulent", "cactus", "fern", "moss",
        "herb", "vegetable", "fruit", "garden", "foliage", "bonsai",
        "flowerpot", "vase", "seedling", "vine"
    ]

    /// Below this nothing counts as evidence. Apple's classifier returns a long
    /// tail of near-zero labels and treating those as a plant would defeat the
    /// gate entirely.
    static let floor = 0.1

    /// A label this strong is trusted on its own.
    static let confident = 0.35

    enum Verdict: Sendable, Equatable {
        case plant
        case notPlant
    }

    static func verdict(for labels: [CoarseLabel]) -> Verdict {
        plantConfidence(in: labels) >= confident ? .plant : .notPlant
    }

    /// The strongest plant-ish label, which is what the verdict turns on and
    /// what the UI shows when it refuses.
    static func plantConfidence(in labels: [CoarseLabel]) -> Double {
        labels
            .filter { $0.confidence >= floor && isPlantLike($0.identifier) }
            .map(\.confidence)
            .max() ?? 0
    }

    static func isPlantLike(_ identifier: String) -> Bool {
        identifier
            .lowercased()
            .split(whereSeparator: { $0 == "_" || $0 == " " || $0 == "-" })
            .contains { plantWords.contains(String($0)) }
    }

    /// What to show a person, strongest first and capped: the tail is noise.
    static func presentable(_ labels: [CoarseLabel], limit: Int = 5) -> [CoarseLabel] {
        labels
            .filter { $0.confidence >= floor }
            .sorted { $0.confidence > $1.confidence }
            .prefix(limit)
            .map { $0 }
    }
}
