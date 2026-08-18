import Foundation

protocol IdentificationCaching: Sendable {
    func result(for assetID: String) async -> PlantIdentification?
    func store(_ result: PlantIdentification) async
    func clear() async
}

/// Keyed on the asset's localIdentifier, which is meaningless on another
/// device, so this is a device-local cache and never syncs anywhere.
actor FileIdentificationCache: IdentificationCaching {
    private let url: URL
    private var entries: [String: PlantIdentification]?

    init(filename: String = "identifications.json") {
        let base = URL.applicationSupportDirectory
        try? FileManager.default.createDirectory(at: base, withIntermediateDirectories: true)
        url = base.appending(path: filename)
    }

    func result(for assetID: String) async -> PlantIdentification? {
        loaded()[assetID]
    }

    /// Identifying the same asset twice costs a species call and returns the
    /// same answer, so a write is worth more than a read is expensive.
    func store(_ result: PlantIdentification) async {
        var all = loaded()
        all[result.assetID] = result
        entries = all
        persist(all)
    }

    func clear() async {
        entries = [:]
        try? FileManager.default.removeItem(at: url)
    }

    private func loaded() -> [String: PlantIdentification] {
        if let entries { return entries }

        // A cache that cannot be read is empty, not broken: an old shape after
        // an upgrade must not take the feature down with it.
        let decoded = (try? Data(contentsOf: url))
            .flatMap { try? JSONDecoder().decode([String: PlantIdentification].self, from: $0) }
            ?? [:]
        entries = decoded
        return decoded
    }

    private func persist(_ all: [String: PlantIdentification]) {
        guard let data = try? JSONEncoder().encode(all) else { return }
        try? data.write(to: url, options: .atomic)
    }
}

/// In memory, for tests and for a run with no asset to key on.
actor MemoryIdentificationCache: IdentificationCaching {
    private var entries: [String: PlantIdentification] = [:]

    func result(for assetID: String) async -> PlantIdentification? { entries[assetID] }
    func store(_ result: PlantIdentification) async { entries[result.assetID] = result }
    func clear() async { entries = [:] }
}
