import Foundation
import Testing

@testable import Planty

@Suite("Decoding what the Go service marshals")
struct ModelDecodingTests {
    private func decode<T: Decodable>(_ type: T.Type, _ json: String) throws -> T {
        try PlantyCoders.decoder().decode(type, from: Data(json.utf8))
    }

    @Test("A full plant, snake_case keys and all")
    func plant() throws {
        let plant = try decode(Plant.self, Self.monaJSON)
        #expect(plant.slug == "mona")
        #expect(plant.commonName == "Mona")
        #expect(plant.domain == .houseplant)
        #expect(plant.wateringMethod == .hand)
        #expect(plant.accessibility == .awkward)
        #expect(plant.lightExposure == .brightIndirect)
        #expect(plant.minTempF == 50)
        #expect(plant.hasDrainage == true)
        #expect(plant.careProfile.moistureLow == 0.3)
        #expect(plant.careProfile.ownerSays == "Do not let it sit in water.")
    }

    @Test("Ownership comes from steward, and 'self' is not a friend")
    func ownership() throws {
        let friends = try decode(Plant.self, Self.monaJSON)
        #expect(friends.isFriends)
        #expect(friends.ownershipLabel == "Maya's plant")
        #expect(friends.ownershipAccessibilityLabel == "Friend's plant, belongs to Maya")

        let mine = Plant.fixture(steward: "self")
        #expect(!mine.isFriends)
        #expect(mine.ownershipLabel == "Mine")
    }

    @Test("An enum case the app has never heard of does not fail the list")
    func unknownEnumFallsBack() throws {
        let json = Self.monaJSON.replacingOccurrences(
            of: "\"domain\": \"houseplant\"",
            with: "\"domain\": \"hydroponic_mushroom\""
        )
        let plant = try decode(Plant.self, json)
        #expect(plant.domain == .unknown)
        #expect(plant.commonName == "Mona")
    }

    @Test("The digest, including the stale marker")
    func digest() throws {
        let digest = try decode(Digest.self, Self.digestJSON)
        #expect(digest.checked == 8)
        #expect(digest.entries.count == 1)
        #expect(digest.entries[0].verdict.action == .water)
        #expect(digest.entries[0].risk == 5)
        #expect(digest.staleSince == nil)
        #expect(digest.isAllClear == false)
    }

    @Test("A digest carrying stale_since is never all clear, even with no entries")
    func staleDigestIsNotAllClear() throws {
        let json = Self.digestJSON
            .replacingOccurrences(of: "\"entries\": [", with: "\"ignored\": [")
            .replacingOccurrences(of: "\"checked\": 8", with: """
                "checked": 8, "entries": [], "stale_since": "2026-08-15T08:00:00Z"
                """)
        let digest = try decode(Digest.self, json)
        #expect(digest.entries.isEmpty)
        #expect(digest.staleSince != nil)
        #expect(digest.isAllClear == false)
    }

    @Test("Timeline keys are all optional so a partial service still renders")
    func partialTimeline() throws {
        let timeline = try decode(PlantTimeline.self, #"{"photos": []}"#)
        #expect(timeline.isEmpty)
        #expect(timeline.observations.isEmpty)
        #expect(timeline.verdicts.isEmpty)
    }

    @Test("The plant detail envelope the service returns today")
    func plantDetail() throws {
        let json = """
            {
              "plant": \(Self.monaJSON),
              "risk": 5,
              "observations": [],
              "last_watered": "2026-08-16T09:12:00Z"
            }
            """
        let detail = try decode(PlantDetail.self, json)
        #expect(detail.risk == 5)
        #expect(detail.lastWatered != nil)
        #expect(detail.verdict == nil)
    }

    @Test("The service's structured error body")
    func errorBody() throws {
        let body = try decode(
            APIErrorBody.self,
            #"{"error":"The service could not complete the request.","code":"internal_error","request_id":"req-123"}"#
        )
        #expect(body.error == "The service could not complete the request.")
        #expect(body.code == "internal_error")
        #expect(body.requestId == "req-123")
    }

    @Test("A sensor with no baselines can never drive a decision")
    func uncalibratedSensor() throws {
        let json = """
            {
              "id": "8B7B4A0E-3B9C-4E2A-9A6F-0F1C2D3E4A5B",
              "plant_id": "0651DE3F-6EC5-4A7B-9981-8CF8F53D0F4D",
              "ha_entity_id": "sensor.mona_soil",
              "role": "soil_moisture",
              "created_at": "2026-08-01T00:00:00Z"
            }
            """
        let link = try decode(SensorLink.self, json)
        #expect(!link.isCalibrated)
        #expect(link.fraction(of: 42) == nil)
    }

    @Test("Calibration is relative to that one probe's own baselines")
    func calibratedFraction() throws {
        let json = """
            {
              "id": "8B7B4A0E-3B9C-4E2A-9A6F-0F1C2D3E4A5B",
              "plant_id": "0651DE3F-6EC5-4A7B-9981-8CF8F53D0F4D",
              "ha_entity_id": "sensor.mona_soil",
              "role": "soil_moisture",
              "dry_baseline": 200,
              "wet_baseline": 600,
              "calibrated_at": "2026-08-01T00:00:00Z",
              "created_at": "2026-08-01T00:00:00Z"
            }
            """
        let link = try decode(SensorLink.self, json)
        #expect(link.isCalibrated)
        #expect(link.fraction(of: 400) == 0.5)
        #expect(link.fraction(of: 100) == 0)
        #expect(link.fraction(of: 900) == 1)
    }

    @Test("A posted observation encodes the keys the service reads")
    func encodesObservation() throws {
        let encoder = PlantyCoders.encoder()
        encoder.outputFormatting = .sortedKeys
        let data = try encoder.encode(
            NewObservation(kind: .watered, body: "slow drink", occurredAt: .reference)
        )
        let json = try #require(String(bytes: data, encoding: .utf8))
        #expect(json.contains("\"kind\":\"watered\""))
        #expect(json.contains("\"occurred_at\""))
        #expect(json.contains("\"source\":\"app\""))
    }
}

extension ModelDecodingTests {
    static let monaJSON = """
        {
          "id": "0651DE3F-6EC5-4A7B-9981-8CF8F53D0F4D",
          "slug": "mona",
          "common_name": "Mona",
          "botanical_name": "Monstera deliciosa",
          "domain": "houseplant",
          "steward": "Maya",
          "status": "alive",
          "location": "Living room",
          "ha_area": "living_room",
          "accessibility": "awkward",
          "watering_method": "hand",
          "pot_size_in": 10,
          "pot_material": "terracotta",
          "has_drainage": true,
          "light_exposure": "bright_indirect",
          "min_temp_f": 50,
          "care_profile": {
            "owner_says": "Do not let it sit in water.",
            "moisture_low": 0.3,
            "moisture_high": 0.7
          },
          "created_at": "2026-07-01T12:00:00Z",
          "updated_at": "2026-08-17T12:00:00.123456789Z"
        }
        """

    static let digestJSON = """
        {
          "date": "2026-08-18T08:04:00Z",
          "checked": 8,
          "entries": [
            {
              "plant": \(monaJSON),
              "verdict": {
                "id": "1C1B1A19-1817-1615-1413-121110090807",
                "plant_id": "0651DE3F-6EC5-4A7B-9981-8CF8F53D0F4D",
                "for_date": "2026-08-18T00:00:00Z",
                "action": "water",
                "reasoning": "The soil has stayed dry for two days.",
                "confidence": 0.82,
                "evidence": {
                  "photo_ids": ["2C2B2A29-2827-2625-2423-222120191817"],
                  "sensor_summary": "Soil at 12 percent of this probe's range."
                },
                "created_at": "2026-08-18T08:04:00Z"
              },
              "risk": 5
            }
          ]
        }
        """
}
