import Foundation
import Testing

@testable import Planty

/// The two endpoints split the story between them: the timeline has the
/// photos, the plant has the reasons. Reading only one of them is why the story
/// tab showed pictures with nothing said about any of them.
@Suite("Timeline and detail merge")
struct TimelineMergeTests {
    private func photo(_ daysAgo: Int, url: URL? = nil) -> Photo {
        Photo(
            id: UUID(),
            plantID: UUID(),
            storageKey: "k\(daysAgo)",
            takenAt: Date(timeIntervalSince1970: 1_700_000_000 - Double(daysAgo) * 86_400),
            createdAt: Date(timeIntervalSince1970: 1_700_000_000),
            url: url
        )
    }

    private func observation(_ kind: ObservationKind) -> PlantObservation {
        PlantObservation(
            id: UUID(),
            plantID: UUID(),
            kind: kind,
            occurredAt: Date(timeIntervalSince1970: 1_700_000_000),
            source: .app,
            createdAt: Date(timeIntervalSince1970: 1_700_000_000)
        )
    }

    @Test("The plant's observations reach a timeline that has only photos")
    func takesObservationsFromDetail() {
        let timeline = PlantTimeline(photos: [photo(1)])
        var detail = PlantDetail(plant: .fixture())
        detail.observations = [observation(.watered), observation(.repotted)]

        let merged = timeline.merging(detail)

        #expect(merged.photos.count == 1)
        #expect(merged.observations.count == 2)
    }

    @Test("The current verdict becomes the story's verdict when it has none")
    func takesTheVerdictFromDetail() {
        var detail = PlantDetail(plant: .fixture())
        detail.verdict = .fixture()

        #expect(PlantTimeline().merging(detail).verdicts.count == 1)
    }

    /// The evidence disclosure needs both halves, and both only ever arrive on
    /// the plant endpoint, so it was permanently empty.
    @Test("Sensor evidence needs the links and the readings together")
    func buildsSeriesFromDetail() {
        let link = SensorLink(
            id: UUID(),
            haEntityID: "sensor.mona_moisture",
            role: .soilMoisture,
            dryBaseline: 300,
            wetBaseline: 700,
            createdAt: Date()
        )
        var detail = PlantDetail(plant: .fixture())
        detail.sensors = [link]
        detail.readings = [
            Reading(id: UUID(), sensorLinkID: link.id, value: 500, takenAt: Date())
        ]

        let merged = PlantTimeline().merging(detail)

        #expect(merged.series.count == 1)
        #expect(merged.series.first?.latest?.value == 500)
    }

    /// The timeline's photos carry a presigned link and the plant's do not, so
    /// a merge that preferred the plant's would render placeholders.
    @Test("Photos that came with a link are kept over ones without")
    func prefersTimelinePhotos() {
        let linked = photo(1, url: URL(string: "https://example.test/a.jpg"))
        var detail = PlantDetail(plant: .fixture())
        detail.photos = [photo(9)]

        let merged = PlantTimeline(photos: [linked]).merging(detail)

        #expect(merged.photos.count == 1)
        #expect(merged.photos.first?.url != nil)
    }

    /// This shipped. The service sent `"entries": null` on a garden nothing had
    /// judged, the app required an array, and the Today tab could not load at
    /// all: the first thing the app does on launch was the thing that broke.
    @Test("A null entries list decodes as empty rather than failing the digest")
    func decodesNullEntries() throws {
        let json = """
            {
              "date": "2026-08-18T20:23:49Z",
              "entries": null,
              "checked": 5,
              "stale_since": null,
              "never_run": true
            }
            """
        let digest = try PlantyCoders.decoder().decode(Digest.self, from: Data(json.utf8))

        #expect(digest.entries.isEmpty)
        #expect(digest.checked == 5)
    }

    /// A garden nobody has looked at is not a calm one, and rendering it as
    /// calm is the exact reassurance this app must never give.
    @Test("Never having run is not all clear")
    func neverRunIsNotCalm() throws {
        let json = """
            {"date": "2026-08-18T20:23:49Z", "entries": [], "checked": 5, "never_run": true}
            """
        let digest = try PlantyCoders.decoder().decode(Digest.self, from: Data(json.utf8))

        #expect(digest.neverRun)
        #expect(!digest.isAllClear)
    }

    @Test("Having run and found nothing is all clear")
    func ranAndFoundNothingIsCalm() throws {
        let json = """
            {"date": "2026-08-18T20:23:49Z", "entries": [], "checked": 5, "never_run": false}
            """
        let digest = try PlantyCoders.decoder().decode(Digest.self, from: Data(json.utf8))

        #expect(digest.isAllClear)
    }

    @Test("A photo decodes the link the timeline minted for it")
    func decodesPresignedURL() throws {
        let json = """
            {
              "id": "\(UUID().uuidString)",
              "plant_id": "\(UUID().uuidString)",
              "storage_key": "plants/mona/1.jpg",
              "taken_at": "2026-08-18T09:00:00Z",
              "created_at": "2026-08-18T09:00:01Z",
              "url": "https://minio.test/plants/mona/1.jpg?X-Amz-Signature=abc"
            }
            """
        let decoded = try PlantyCoders.decoder().decode(Photo.self, from: Data(json.utf8))

        #expect(decoded.url?.host() == "minio.test")
    }

    @Test("A photo with no link still decodes, because only the timeline mints one")
    func decodesWithoutURL() throws {
        let json = """
            {
              "id": "\(UUID().uuidString)",
              "plant_id": "\(UUID().uuidString)",
              "storage_key": "plants/mona/1.jpg",
              "taken_at": "2026-08-18T09:00:00Z",
              "created_at": "2026-08-18T09:00:01Z"
            }
            """
        let decoded = try PlantyCoders.decoder().decode(Photo.self, from: Data(json.utf8))

        #expect(decoded.url == nil)
    }
}
