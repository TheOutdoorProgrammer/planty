import CryptoKit
import Foundation
import ImageIO
import UniformTypeIdentifiers

enum ImageRendition: String, Sendable {
    case thumbnail
    case original
}

/// Stable identity and content version are deliberately separate from the
/// short-lived signed URL used to fetch the bytes.
struct ImageCacheKey: Hashable, Sendable {
    let identity: String
    let rendition: ImageRendition
    let version: String

    static func photo(_ photo: Photo, rendition: ImageRendition) -> ImageCacheKey {
        ImageCacheKey(
            identity: "photo:\(photo.id.uuidString)",
            rendition: rendition,
            version: "\(photo.createdAt.timeIntervalSince1970):\(photo.storageKey)"
        )
    }

    /// The list response does not expose the cover's Photo ID. Plant identity
    /// plus the cover timestamp still changes exactly when the visible photo
    /// does, without tying cached bytes to a rotating signed URL.
    static func cover(_ plant: Plant, rendition: ImageRendition) -> ImageCacheKey {
        cover(
            slug: plant.slug,
            changedAt: plant.photoTakenAt ?? plant.updatedAt,
            rendition: rendition
        )
    }

    /// Uploads know the plant slug and capture time before the next list load,
    /// which lets them seed the exact cover key that response will use.
    static func cover(
        slug: String,
        changedAt: Date,
        rendition: ImageRendition
    ) -> ImageCacheKey {
        ImageCacheKey(
            identity: "cover:\(slug)",
            rendition: rendition,
            version: String(changedAt.timeIntervalSince1970)
        )
    }

    fileprivate var filename: String {
        let value = "\(identity)\u{0}\(rendition.rawValue)\u{0}\(version)"
        let digest = SHA256.hash(data: Data(value.utf8))
        return digest.map { String(format: "%02x", $0) }.joined() + ".image"
    }
}

enum ImageRepositoryError: Error, Equatable, Sendable {
    case unsupportedURL
    case invalidResponse
    case invalidImage
}

/// One cache for thumbnails and originals. The actor owns the memory cache,
/// disk index and in-flight tasks so parallel views cannot race or redownload.
actor ImageRepository {
    typealias Loader = @Sendable (URL) async throws -> Data

    static let thumbnailMaximumPixelSize = 1_200

    private struct Pending {
        let token: UUID
        let task: Task<Data, Error>
    }

    private let memory = NSCache<NSString, NSData>()
    private let cacheDirectory: URL
    private let maximumDiskBytes: Int
    private let loader: Loader
    private var inFlight: [ImageCacheKey: Pending] = [:]

    init(
        cacheDirectory: URL? = nil,
        maximumMemoryBytes: Int = 32 * 1_024 * 1_024,
        maximumDiskBytes: Int = 200 * 1_024 * 1_024,
        loader: @escaping Loader = ImageRepository.download
    ) {
        let base = cacheDirectory ?? FileManager.default
            .urls(for: .cachesDirectory, in: .userDomainMask)[0]
            .appending(path: "Planty/Images", directoryHint: .isDirectory)
        self.cacheDirectory = base
        self.maximumDiskBytes = max(0, maximumDiskBytes)
        self.loader = loader
        memory.totalCostLimit = max(0, maximumMemoryBytes)
        try? FileManager.default.createDirectory(
            at: base,
            withIntermediateDirectories: true
        )
    }

    func data(for key: ImageCacheKey, from url: URL) async throws -> Data {
        guard url.validatedRemoteImageURL != nil else {
            throw ImageRepositoryError.unsupportedURL
        }
        try Task.checkCancellation()

        if let cached = memory.object(forKey: key.filename as NSString) {
            return cached as Data
        }
        if let cached = readDisk(key) {
            memory.setObject(cached as NSData, forKey: key.filename as NSString, cost: cached.count)
            return cached
        }

        let pending: Pending
        if let existing = inFlight[key] {
            pending = existing
        } else {
            let token = UUID()
            let loader = self.loader
            let rendition = key.rendition
            let task = Task {
                let downloaded = try await loader(url)
                return try Self.prepared(downloaded, rendition: rendition)
            }
            pending = Pending(token: token, task: task)
            inFlight[key] = pending
        }

        do {
            let loaded = try await pending.task.value
            if inFlight[key]?.token == pending.token {
                inFlight.removeValue(forKey: key)
                storePrepared(loaded, for: key)
            }
            try Task.checkCancellation()
            return loaded
        } catch {
            if inFlight[key]?.token == pending.token {
                inFlight.removeValue(forKey: key)
            }
            throw error
        }
    }

    /// Uploads already have trustworthy local bytes. Seeding both renditions
    /// makes the next list load immediate even if its signed URL has changed.
    func seed(_ data: Data, for key: ImageCacheKey) {
        guard let prepared = try? Self.prepared(data, rendition: key.rendition) else { return }
        storePrepared(prepared, for: key)
    }

    func clearMemory() {
        memory.removeAllObjects()
    }

    /// Internal diagnostics used by focused cache tests.
    func diskURL(for key: ImageCacheKey) -> URL {
        cacheDirectory.appending(path: key.filename)
    }

    func diskUsage() -> (files: Int, bytes: Int) {
        let entries = diskEntries()
        return (entries.count, entries.reduce(0) { $0 + $1.size })
    }

    private func readDisk(_ key: ImageCacheKey) -> Data? {
        let url = cacheDirectory.appending(path: key.filename)
        guard let data = try? Data(contentsOf: url) else { return nil }
        guard Self.isImage(data) else {
            try? FileManager.default.removeItem(at: url)
            return nil
        }
        try? FileManager.default.setAttributes(
            [.modificationDate: Date()],
            ofItemAtPath: url.path
        )
        return data
    }

    private func storePrepared(_ data: Data, for key: ImageCacheKey) {
        memory.setObject(data as NSData, forKey: key.filename as NSString, cost: data.count)
        guard maximumDiskBytes > 0 else { return }
        let url = cacheDirectory.appending(path: key.filename)
        try? data.write(to: url, options: .atomic)
        pruneDisk()
    }

    private func pruneDisk() {
        var entries = diskEntries().sorted {
            if $0.modified != $1.modified { return $0.modified < $1.modified }
            return $0.url.lastPathComponent < $1.url.lastPathComponent
        }
        var total = entries.reduce(0) { $0 + $1.size }
        while total > maximumDiskBytes, let oldest = entries.first {
            try? FileManager.default.removeItem(at: oldest.url)
            total -= oldest.size
            entries.removeFirst()
        }
    }

    private func diskEntries() -> [(url: URL, size: Int, modified: Date)] {
        let keys: Set<URLResourceKey> = [.fileSizeKey, .contentModificationDateKey, .isRegularFileKey]
        let urls = (try? FileManager.default.contentsOfDirectory(
            at: cacheDirectory,
            includingPropertiesForKeys: Array(keys),
            options: [.skipsHiddenFiles]
        )) ?? []
        return urls.compactMap { url in
            guard let values = try? url.resourceValues(forKeys: keys),
                  values.isRegularFile == true
            else { return nil }
            return (url, values.fileSize ?? 0, values.contentModificationDate ?? .distantPast)
        }
    }

    private static func prepared(_ data: Data, rendition: ImageRendition) throws -> Data {
        guard isImage(data) else { throw ImageRepositoryError.invalidImage }
        guard rendition == .thumbnail,
              let source = CGImageSourceCreateWithData(data as CFData, nil),
              let image = CGImageSourceCreateThumbnailAtIndex(
                source,
                0,
                [
                    kCGImageSourceCreateThumbnailFromImageAlways: true,
                    kCGImageSourceCreateThumbnailWithTransform: true,
                    kCGImageSourceThumbnailMaxPixelSize: thumbnailMaximumPixelSize
                ] as CFDictionary
              )
        else { return data }

        let output = NSMutableData()
        guard let destination = CGImageDestinationCreateWithData(
            output,
            UTType.jpeg.identifier as CFString,
            1,
            nil
        ) else { throw ImageRepositoryError.invalidImage }
        CGImageDestinationAddImage(
            destination,
            image,
            [kCGImageDestinationLossyCompressionQuality: 0.82] as CFDictionary
        )
        guard CGImageDestinationFinalize(destination) else {
            throw ImageRepositoryError.invalidImage
        }
        return output as Data
    }

    private static func isImage(_ data: Data) -> Bool {
        guard !data.isEmpty,
              let source = CGImageSourceCreateWithData(data as CFData, nil)
        else { return false }
        return CGImageSourceGetCount(source) > 0
    }

    private static func download(_ url: URL) async throws -> Data {
        guard url.validatedRemoteImageURL != nil else {
            throw ImageRepositoryError.unsupportedURL
        }
        let (data, response) = try await URLSession.plantyDefault.data(from: url)
        guard let http = response as? HTTPURLResponse,
              (200..<300).contains(http.statusCode)
        else { throw ImageRepositoryError.invalidResponse }
        return data
    }
}
