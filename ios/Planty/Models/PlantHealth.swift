import Foundation

struct HealthEvidence: Codable, Sendable, Hashable {
    var photoIDs: [UUID]
    var observationIDs: [UUID]
    var readingIDs: [UUID]
    var summary: String?
    var modelVersion: String?

    init(
        photoIDs: [UUID] = [],
        observationIDs: [UUID] = [],
        readingIDs: [UUID] = [],
        summary: String? = nil,
        modelVersion: String? = nil
    ) {
        self.photoIDs = photoIDs
        self.observationIDs = observationIDs
        self.readingIDs = readingIDs
        self.summary = summary
        self.modelVersion = modelVersion
    }

    enum CodingKeys: String, CodingKey {
        case photoIDs = "photo_ids"
        case observationIDs = "observation_ids"
        case readingIDs = "reading_ids"
        case summary
        case modelVersion = "model_version"
    }

    init(from decoder: any Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        photoIDs = try container.decodeIfPresent([UUID].self, forKey: .photoIDs) ?? []
        observationIDs = try container.decodeIfPresent([UUID].self, forKey: .observationIDs) ?? []
        readingIDs = try container.decodeIfPresent([UUID].self, forKey: .readingIDs) ?? []
        summary = try container.decodeIfPresent(String.self, forKey: .summary)
        modelVersion = try container.decodeIfPresent(String.self, forKey: .modelVersion)
    }
}

struct HealthEvent: Codable, Sendable, Hashable, Identifiable {
    let id: UUID
    let plantID: UUID
    let score: Double
    let requestedDelta: Double?
    let appliedDelta: Double?
    let rationale: String
    let evidence: HealthEvidence
    let source: ObservationSource
    let actor: String?
    let judgmentRunID: UUID?
    let idempotencyKey: UUID?
    let createdAt: Date

    var isBaseline: Bool { requestedDelta == nil }

    enum CodingKeys: String, CodingKey {
        case id
        case plantID = "plant_id"
        case score
        case requestedDelta = "requested_delta"
        case appliedDelta = "applied_delta"
        case rationale
        case evidence
        case source
        case actor
        case judgmentRunID = "judgment_run_id"
        case idempotencyKey = "idempotency_key"
        case createdAt = "created_at"
    }
}

struct PlantHealthResponse: Codable, Sendable, Hashable {
    var current: HealthEvent?
    var events: [HealthEvent]
    var count: Int

    static let unknown = PlantHealthResponse(current: nil, events: [], count: 0)
}

struct NewHealthChange: Codable, Sendable, Hashable {
    let kind: HealthChangeKind
    let value: Double
    let rationale: String
    let evidence: HealthEvidence
    let actor: String?
    let idempotencyKey: UUID

    enum CodingKeys: String, CodingKey {
        case kind
        case value
        case rationale
        case evidence
        case actor
        case idempotencyKey = "idempotency_key"
    }
}

struct HealthAdjustmentDraft: Equatable, Sendable {
    let kind: HealthChangeKind
    var value = ""
    var rationale = ""
    var evidenceSummary = ""

    func request(idempotencyKey: UUID, actor: String = "owner") -> NewHealthChange? {
        let rationale = rationale.cleaned
        let evidence = evidenceSummary.cleaned
        guard let value = Double(value.cleaned), value.isFinite,
              !rationale.isEmpty, !evidence.isEmpty
        else { return nil }

        switch kind {
        case .baseline where !(0...100).contains(value): return nil
        case .delta where value == 0: return nil
        default: break
        }

        return NewHealthChange(
            kind: kind,
            value: value,
            rationale: rationale,
            evidence: HealthEvidence(summary: evidence),
            actor: actor,
            idempotencyKey: idempotencyKey
        )
    }
}

struct HealthPresentation: Equatable, Sendable {
    let score: Double?

    var title: String {
        guard let score else { return "Health unknown" }
        return "Health evidence: \(Self.number(score)) out of 100"
    }

    var accessibilityDescription: String {
        guard let score else {
            return "Health unknown. No baseline has been recorded."
        }
        if score == 0 {
            return "Health evidence is zero out of 100, meaning dead or unrecoverable. Archiving still requires separate confirmation."
        }
        if score == 100 {
            return "Health evidence is 100 out of 100, meaning no known or visible health deficit."
        }
        return "Health evidence is \(Self.number(score)) out of 100."
    }

    private static func number(_ value: Double) -> String {
        value.formatted(.number.precision(.fractionLength(0...1)))
    }
}
