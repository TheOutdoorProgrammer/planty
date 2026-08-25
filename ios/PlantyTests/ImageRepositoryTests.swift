import Foundation
import Testing

@testable import Planty

private actor ImageLoadProbe {
    private(set) var calls = 0
    let data: Data
    let delay: Duration

    init(data: Data, delay: Duration = .zero) {
        self.data = data
        self.delay = delay
    }

    func load(_ url: URL) async throws -> Data {
        calls += 1
        if delay > .zero {
            try await Task.sleep(for: delay)
        }
        return data
    }
}

private actor CancelingImageLoadProbe {
    private(set) var calls = 0
    let data: Data

    init(data: Data) {
        self.data = data
    }

    func load(_ url: URL) async throws -> Data {
        calls += 1
        if calls == 1 { throw CancellationError() }
        return data
    }
}

@Suite("Image repository")
struct ImageRepositoryTests {
    private let image = Data(base64Encoded: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+A8AAQUBAScY42YAAAAASUVORK5CYII=")!
    private let firstURL = URL(string: "https://photos.example/first?signature=one")!
    private let rotatedURL = URL(string: "https://photos.example/second?signature=two")!

    private func directory() -> URL {
        FileManager.default.temporaryDirectory
            .appending(path: "planty-image-tests-\(UUID().uuidString)", directoryHint: .isDirectory)
    }

    private func key(_ suffix: String = "one") -> ImageCacheKey {
        ImageCacheKey(identity: "photo:\(suffix)", rendition: .original, version: "v1")
    }

    @Test("A memory hit does not call the loader twice")
    func memoryHit() async throws {
        let folder = directory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let probe = ImageLoadProbe(data: image)
        let repository = ImageRepository(cacheDirectory: folder, maximumDiskBytes: 0) {
            try await probe.load($0)
        }

        _ = try await repository.data(for: key(), from: firstURL)
        let second = try await repository.data(for: key(), from: rotatedURL)

        #expect(second == image)
        #expect(await probe.calls == 1)
    }

    @Test("A disk hit survives repository recreation and a rotated signed URL")
    func diskHitUsesStableKey() async throws {
        let folder = directory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let firstProbe = ImageLoadProbe(data: image)
        let first = ImageRepository(cacheDirectory: folder) { try await firstProbe.load($0) }
        _ = try await first.data(for: key(), from: firstURL)

        let secondProbe = ImageLoadProbe(data: image)
        let recreated = ImageRepository(cacheDirectory: folder) { try await secondProbe.load($0) }
        let loaded = try await recreated.data(for: key(), from: rotatedURL)

        #expect(loaded == image)
        #expect(await firstProbe.calls == 1)
        #expect(await secondProbe.calls == 0)
    }

    @Test("Concurrent callers share one download")
    func coalescesRequests() async throws {
        let folder = directory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let probe = ImageLoadProbe(data: image, delay: .milliseconds(80))
        let repository = ImageRepository(cacheDirectory: folder) { try await probe.load($0) }

        async let first = repository.data(for: key(), from: firstURL)
        async let second = repository.data(for: key(), from: rotatedURL)
        let values = try await [first, second]

        #expect(values == [image, image])
        #expect(await probe.calls == 1)
    }

    @Test("A corrupt disk entry is evicted and downloaded again")
    func corruptEntryIsEvicted() async throws {
        let folder = directory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let probe = ImageLoadProbe(data: image)
        let repository = ImageRepository(cacheDirectory: folder) { try await probe.load($0) }
        let diskURL = await repository.diskURL(for: key())
        try Data("not an image".utf8).write(to: diskURL, options: .atomic)

        let loaded = try await repository.data(for: key(), from: firstURL)

        #expect(loaded == image)
        #expect(await probe.calls == 1)
        #expect((try? Data(contentsOf: diskURL)) == image)
    }

    @Test("Canceling one waiter does not cancel or poison the shared load")
    func cancellationKeepsSharedLoad() async throws {
        let folder = directory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let probe = ImageLoadProbe(data: image, delay: .milliseconds(80))
        let repository = ImageRepository(cacheDirectory: folder) { try await probe.load($0) }
        let canceled = Task { try await repository.data(for: key(), from: firstURL) }
        try await Task.sleep(for: .milliseconds(10))
        canceled.cancel()

        do {
            _ = try await canceled.value
            Issue.record("The canceled waiter unexpectedly received an image")
        } catch is CancellationError {
            // The shared task completes and is cached even though this waiter
            // no longer wants its result.
        }

        let loaded = try await repository.data(for: key(), from: rotatedURL)
        #expect(loaded == image)
        #expect(await probe.calls == 1)
    }

    @Test("The disk cache removes old entries to stay under its byte bound")
    func sizeEviction() async throws {
        let folder = directory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let repository = ImageRepository(
            cacheDirectory: folder,
            maximumDiskBytes: image.count + 1
        )

        await repository.seed(image, for: key("first"))
        try await Task.sleep(for: .milliseconds(10))
        await repository.seed(image, for: key("second"))
        let usage = await repository.diskUsage()

        #expect(usage.files == 1)
        #expect(usage.bytes <= image.count + 1)
    }

    @Test("A canceled loader is removed so a later request can retry")
    func loaderCancellationCanRetry() async throws {
        let folder = directory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let probe = CancelingImageLoadProbe(data: image)
        let repository = ImageRepository(cacheDirectory: folder) { try await probe.load($0) }

        await #expect(throws: CancellationError.self) {
            try await repository.data(for: key(), from: firstURL)
        }
        let retried = try await repository.data(for: key(), from: rotatedURL)

        #expect(retried == image)
        #expect(await probe.calls == 2)
    }

    @Test("Unsupported schemes fail before invoking the loader")
    func rejectsUnsupportedSchemes() async {
        let folder = directory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let probe = ImageLoadProbe(data: image)
        let repository = ImageRepository(cacheDirectory: folder) { try await probe.load($0) }
        let fileURL = URL(fileURLWithPath: "/tmp/not-an-image")

        await #expect(throws: ImageRepositoryError.unsupportedURL) {
            try await repository.data(for: key(), from: fileURL)
        }
        #expect(await probe.calls == 0)
    }

    @Test("A successful upload seeds the stable original and thumbnail keys")
    func uploadSeedsCache() async throws {
        let folder = directory()
        defer { try? FileManager.default.removeItem(at: folder) }
        let probe = ImageLoadProbe(data: image)
        let repository = ImageRepository(cacheDirectory: folder) { try await probe.load($0) }
        let transport = IsolatedStubTransport()
        let photoID = UUID()
        let plantID = UUID()
        transport.respond(json: """
            {
              "id":"\(photoID.uuidString)",
              "plant_id":"\(plantID.uuidString)",
              "storage_key":"plants/aloe/original.jpg",
              "taken_at":"2026-08-25T12:00:00Z",
              "created_at":"2026-08-25T12:00:01Z",
              "url":"https://photos.example/uploaded?signature=old"
            }
            """)
        let client = transport.client(images: repository)

        let saved = try await client.uploadPhoto(
            slug: "aloe",
            jpeg: image,
            caption: nil,
            takenAt: Date(timeIntervalSince1970: 1_777_118_400)
        )
        await repository.clearMemory()

        let original = try await repository.data(
            for: .photo(saved, rendition: .original),
            from: rotatedURL
        )
        let thumbnail = try await repository.data(
            for: .photo(saved, rendition: .thumbnail),
            from: rotatedURL
        )
        let cover = try await repository.data(
            for: .cover(slug: "aloe", changedAt: saved.takenAt, rendition: .thumbnail),
            from: rotatedURL
        )

        #expect(original == image)
        #expect(!thumbnail.isEmpty)
        #expect(!cover.isEmpty)
        #expect(await probe.calls == 0)
    }
}
