import Foundation
import Testing

@testable import Planty

private struct FakeAnalyzer: ImageAnalyzing {
    var labels: [CoarseLabel] = []
    var cutout: Data?
    var isolateFails = false
    var classifyFails = false

    func isolateSubject(in data: Data) async throws -> Data? {
        if isolateFails { throw PlantyError.transport("no gpu") }
        return cutout
    }

    func classify(_ data: Data) async throws -> [CoarseLabel] {
        if classifyFails { throw PlantyError.transport("vision down") }
        return labels
    }
}

private struct FakeIdentifier: PlantIdentifying {
    var backendID = "fake/v1"
    var candidates: [IdentificationCandidate] = []
    var failure: PlantyError?

    /// Records what the species backend was actually handed, which is how the
    /// cutout and the metadata are proven to reach it.
    final class Seen: @unchecked Sendable {
        var data: Data?
        var metadata: CaptureMetadata?
    }
    var seen = Seen()

    func identify(imageData: Data, metadata: CaptureMetadata) async throws -> [IdentificationCandidate] {
        seen.data = imageData
        seen.metadata = metadata
        if let failure { throw failure }
        return candidates
    }
}

@Suite("Identification pipeline")
struct IdentificationPipelineTests {
    private let epoch = Date(timeIntervalSince1970: 1_700_000_000)

    private func candidate(_ name: String, _ confidence: Double) -> IdentificationCandidate {
        IdentificationCandidate(commonName: name, scientificName: nil, confidence: confidence)
    }

    private func pipeline(
        analyzer: FakeAnalyzer,
        identifier: FakeIdentifier,
        cache: any IdentificationCaching = MemoryIdentificationCache()
    ) -> IdentificationPipeline {
        IdentificationPipeline(
            intake: PickerOnlyIntake(),
            analyzer: analyzer,
            identifier: identifier,
            cache: cache,
            now: { self.epoch }
        )
    }

    /// The whole point of the gate: a dog costs nothing.
    @Test("A photo that is not a plant never reaches the species backend")
    func gateStopsNonPlants() async {
        let identifier = FakeIdentifier(candidates: [candidate("Monstera", 0.9)])
        let analyzer = FakeAnalyzer(labels: [CoarseLabel(identifier: "dog", confidence: 0.95)])

        let outcome = await pipeline(analyzer: analyzer, identifier: identifier)
            .identify(pickedData: Data([1, 2, 3]), assetID: nil)

        #expect(outcome == .notAPlant(coarse: PlantGate.presentable(analyzer.labels)))
        #expect(identifier.seen.data == nil)
    }

    @Test("A plant reaches the backend and comes back ranked")
    func identifiesPlants() async throws {
        let identifier = FakeIdentifier(candidates: [
            candidate("Pothos", 0.4), candidate("Monstera", 0.85)
        ])
        let analyzer = FakeAnalyzer(labels: [CoarseLabel(identifier: "houseplant", confidence: 0.9)])

        let outcome = await pipeline(analyzer: analyzer, identifier: identifier)
            .identify(pickedData: Data([1]), assetID: nil)

        let result = try #require(outcome.identification)
        #expect(result.candidates.map(\.commonName) == ["Monstera", "Pothos"])
        #expect(result.backend == "fake/v1")
    }

    /// The cutout is what the backend should judge: indoor photos are cluttered
    /// and the background drags accuracy down.
    @Test("The cutout is what gets classified and sent, not the raw frame")
    func sendsTheCutout() async {
        let raw = Data([9, 9, 9])
        let cutout = Data([1, 1, 1])
        let identifier = FakeIdentifier(candidates: [candidate("Fern", 0.8)])
        let analyzer = FakeAnalyzer(
            labels: [CoarseLabel(identifier: "plant", confidence: 0.9)],
            cutout: cutout
        )

        _ = await pipeline(analyzer: analyzer, identifier: identifier)
            .identify(pickedData: raw, assetID: nil)

        #expect(identifier.seen.data == cutout)
    }

    @Test("No foreground found still classifies the raw frame")
    func fallsBackToTheRawFrame() async {
        let raw = Data([7])
        let identifier = FakeIdentifier(candidates: [candidate("Fern", 0.8)])
        let analyzer = FakeAnalyzer(
            labels: [CoarseLabel(identifier: "plant", confidence: 0.9)],
            cutout: nil,
            isolateFails: true
        )

        _ = await pipeline(analyzer: analyzer, identifier: identifier)
            .identify(pickedData: raw, assetID: nil)

        #expect(identifier.seen.data == raw)
    }

    /// Offline: steps 3 and 4 still run, and the species half degrades to a
    /// stated reason rather than an empty screen.
    @Test("A dead species backend still shows the coarse labels")
    func degradesToCoarse() async {
        let labels = [CoarseLabel(identifier: "houseplant", confidence: 0.9)]
        let identifier = FakeIdentifier(failure: .offline)
        let analyzer = FakeAnalyzer(labels: labels)

        let outcome = await pipeline(analyzer: analyzer, identifier: identifier)
            .identify(pickedData: Data([1]), assetID: nil)

        guard case .coarseOnly(let coarse, let reason) = outcome else {
            Issue.record("expected coarseOnly, got \(outcome)")
            return
        }
        #expect(coarse == PlantGate.presentable(labels))
        #expect(!reason.isEmpty)
    }

    /// Vision failing entirely must not read as "not a plant", which would be a
    /// confident refusal based on nothing.
    @Test("Vision failing does not become a refusal")
    func visionFailureIsNotARefusal() async {
        let identifier = FakeIdentifier(candidates: [candidate("Fern", 0.8)])
        let analyzer = FakeAnalyzer(classifyFails: true)

        let outcome = await pipeline(analyzer: analyzer, identifier: identifier)
            .identify(pickedData: Data([1]), assetID: nil)

        #expect(outcome.identification != nil)
    }

    @Test("An asset is identified once and then served from cache")
    func cachesByAsset() async {
        let cache = MemoryIdentificationCache()
        let identifier = FakeIdentifier(candidates: [candidate("Fern", 0.8)])
        let analyzer = FakeAnalyzer(labels: [CoarseLabel(identifier: "plant", confidence: 0.9)])
        let subject = pipeline(analyzer: analyzer, identifier: identifier, cache: cache)

        _ = await subject.identify(pickedData: Data([1]), assetID: "asset-1")
        identifier.seen.data = nil
        let second = await subject.identify(pickedData: Data([1]), assetID: "asset-1")

        #expect(second.identification?.best?.commonName == "Fern")
        #expect(identifier.seen.data == nil)
    }

    /// A camera capture has no asset, so there is no stable key to cache it
    /// against and re-identifying is the correct cost.
    @Test("A photo with no asset is not cached")
    func doesNotCacheWithoutAnAsset() async {
        let cache = MemoryIdentificationCache()
        let identifier = FakeIdentifier(candidates: [candidate("Fern", 0.8)])
        let analyzer = FakeAnalyzer(labels: [CoarseLabel(identifier: "plant", confidence: 0.9)])

        _ = await pipeline(analyzer: analyzer, identifier: identifier, cache: cache)
            .identify(pickedData: Data([1]), assetID: nil)

        #expect(await cache.result(for: "") == nil)
    }

    @Test("A backend that answers nothing is not a false identification")
    func emptyCandidatesDegrade() async {
        let identifier = FakeIdentifier(candidates: [])
        let analyzer = FakeAnalyzer(labels: [CoarseLabel(identifier: "plant", confidence: 0.9)])

        let outcome = await pipeline(analyzer: analyzer, identifier: identifier)
            .identify(pickedData: Data([1]), assetID: nil)

        #expect(outcome.identification == nil)
    }

    @Test("Confidence is banded rather than shown as false precision")
    func bandsConfidence() {
        #expect(candidate("a", 0.9).strength == .likely)
        #expect(candidate("a", 0.5).strength == .possible)
        #expect(candidate("a", 0.2).strength == .weak)
    }
}
