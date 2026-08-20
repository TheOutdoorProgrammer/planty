import Foundation

struct ManagedChoice: Codable, Hashable, Identifiable, Sendable {
    let value: String
    let sources: [String]
    let lastUsedAt: Date?

    var id: String { value.lowercased() }

    enum CodingKeys: String, CodingKey {
        case value, sources
        case lastUsedAt = "last_used_at"
    }
}

struct ManagedChoiceList: Codable, Hashable, Sendable {
    var recent: [ManagedChoice]
    var all: [ManagedChoice]

    static let empty = ManagedChoiceList(recent: [], all: [])
}

struct ManagedChoices: Codable, Hashable, Sendable {
    var places: ManagedChoiceList
    var owners: ManagedChoiceList
    var potMaterials: ManagedChoiceList

    static let empty = ManagedChoices(places: .empty, owners: .empty, potMaterials: .empty)

    enum CodingKeys: String, CodingKey {
        case places, owners
        case potMaterials = "pot_materials"
    }
}
