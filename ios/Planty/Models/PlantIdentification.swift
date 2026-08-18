import CoreLocation
import Foundation

/// What the original file says about itself. Region and season narrow a species
/// candidate set hard, so this is evidence rather than decoration.
struct CaptureMetadata: Sendable, Equatable, Codable {
    var capturedAt: Date?
    var latitude: Double?
    var longitude: Double?
    var altitude: Double?

    /// A screenshot of somebody else's plant is not a photo of yours, and the
    /// difference is only visible here.
    var isScreenshot = false

    /// True when this came off a PHAsset. The picker hands back a sanitised
    /// copy with GPS commonly stripped, so an absent coordinate means "not
    /// known" rather than "not outdoors".
    var fromOriginal = false

    var coordinate: CLLocationCoordinate2D? {
        guard let latitude, let longitude else { return nil }
        return CLLocationCoordinate2D(latitude: latitude, longitude: longitude)
    }

    var hasLocation: Bool { latitude != nil && longitude != nil }
}

/// One label Apple's own classifier returned. Kept separate from a species
/// candidate: this taxonomy knows "houseplant", never "Monstera deliciosa".
struct CoarseLabel: Sendable, Equatable, Codable, Identifiable {
    let identifier: String
    let confidence: Double

    var id: String { identifier }

    /// Apple returns dotted paths like `plant_flower_rose`. The leaf is the
    /// only part worth showing a person.
    var readable: String {
        identifier
            .split(separator: "_")
            .last
            .map { $0.replacingOccurrences(of: "-", with: " ").capitalized }
            ?? identifier
    }
}

/// One ranked answer to "what is this". Never presented alone: a single
/// confident-looking wrong species is worse than three honest options.
struct IdentificationCandidate: Sendable, Equatable, Codable, Identifiable {
    let commonName: String
    var scientificName: String?
    let confidence: Double

    enum CodingKeys: String, CodingKey {
        case commonName = "common_name"
        case scientificName = "scientific_name"
        case confidence
    }

    var id: String { scientificName ?? commonName }

    /// Bands rather than a percentage. People read 73% as precision that a
    /// species classifier does not have.
    var strength: Strength {
        switch confidence {
        case ..<0.35: .weak
        case ..<0.7: .possible
        default: .likely
        }
    }

    enum Strength: String, Sendable, Codable {
        case weak, possible, likely

        var label: String {
            switch self {
            case .weak: "Long shot"
            case .possible: "Possible"
            case .likely: "Likely"
            }
        }
    }
}

/// A finished run of the pipeline, cached against the asset it came from.
struct PlantIdentification: Sendable, Equatable, Codable {
    /// The PHAsset localIdentifier when there was one. Device-local by nature,
    /// which is why this cache never syncs anywhere.
    let assetID: String

    var candidates: [IdentificationCandidate] = []
    var coarse: [CoarseLabel] = []
    var metadata = CaptureMetadata()

    /// Which backend answered, so a result stays attributable when the backend
    /// changes underneath it.
    let backend: String
    let identifiedAt: Date

    var best: IdentificationCandidate? { candidates.first }
}

/// What `POST /v1/identify` answers.
struct IdentifyResponse: Decodable, Sendable {
    let candidates: [IdentificationCandidate]
}

/// How far the pipeline got. Each case is a real outcome somebody has to be
/// told about, not an error to swallow.
enum IdentificationOutcome: Sendable, Equatable {
    /// The coarse gate says this is not a plant. Instant, offline, free, and
    /// the reason no species call was made.
    case notAPlant(coarse: [CoarseLabel])

    /// The gate passed but the species backend could not be reached. Coarse
    /// labels are still worth showing.
    case coarseOnly(coarse: [CoarseLabel], reason: String)

    case identified(PlantIdentification)

    var identification: PlantIdentification? {
        if case .identified(let result) = self { return result }
        return nil
    }
}
