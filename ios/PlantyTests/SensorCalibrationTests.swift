import Foundation
import Testing

@testable import Planty

/// Calibration is the step that decides whether a probe is allowed to drive
/// watering at all, so the rule that keeps it the right way round is worth more
/// tests than the two numbers suggest.
@Suite("Sensor calibration")
struct SensorCalibrationTests {
    @Test("Wet above dry is the only usable way round")
    func rejectsBackwardsBaselines() {
        #expect(SensorCalibration(dryBaseline: 320, wetBaseline: 720).readsTheRightWayRound)
        #expect(!SensorCalibration(dryBaseline: 720, wetBaseline: 320).readsTheRightWayRound)
    }

    /// Equal baselines divide by zero in `fraction(of:)`, so they are refused
    /// rather than quietly producing an unreadable probe.
    @Test("Two identical readings are not a calibration")
    func rejectsIdenticalBaselines() {
        #expect(!SensorCalibration(dryBaseline: 500, wetBaseline: 500).readsTheRightWayRound)
    }

    @Test("What was typed only becomes a calibration once both boxes are numbers")
    func parsesOnlyCompleteInput() {
        #expect(SensorCalibration(dry: "320", wet: "720") != nil)
        #expect(SensorCalibration(dry: " 320 ", wet: " 720 ") != nil)
        #expect(SensorCalibration(dry: "320", wet: "") == nil)
        #expect(SensorCalibration(dry: "", wet: "720") == nil)
        #expect(SensorCalibration(dry: "wet", wet: "dry") == nil)
    }

    @Test("A calibration sends the baselines under the names Planty knows")
    func encodesSnakeCase() throws {
        let encoded = try JSONEncoder().encode(SensorCalibration(dryBaseline: 320, wetBaseline: 720))
        let json = try #require(
            try JSONSerialization.jsonObject(with: encoded) as? [String: Any]
        )

        #expect(json["dry_baseline"] as? Double == 320)
        #expect(json["wet_baseline"] as? Double == 720)
    }

    /// A probe reading below its own dry baseline is drier than it has ever
    /// been measured, not an error, so it clamps rather than going negative.
    @Test("A reading maps onto this probe's own scale and clamps at both ends")
    func mapsReadingsOntoItsOwnScale() {
        let link = SensorLink(
            id: UUID(),
            haEntityID: "sensor.mona_moisture",
            role: .soilMoisture,
            dryBaseline: 300,
            wetBaseline: 700,
            createdAt: Date()
        )

        #expect(link.fraction(of: 500) == 0.5)
        #expect(link.fraction(of: 100) == 0)
        #expect(link.fraction(of: 900) == 1)
    }

    @Test("An uncalibrated probe maps nothing rather than guessing")
    func refusesToMapWithoutBaselines() {
        let link = SensorLink(
            id: UUID(),
            haEntityID: "sensor.new_probe",
            role: .soilMoisture,
            createdAt: Date()
        )

        #expect(!link.isCalibrated)
        #expect(link.fraction(of: 500) == nil)
    }

    @Test("Only soil moisture requires calibration")
    func calibrationIsRoleSpecific() {
        #expect(SensorRole.soilMoisture.requiresCalibration)
        #expect(!SensorRole.ambientTemp.requiresCalibration)
        #expect(!SensorRole.ambientHumidity.requiresCalibration)
        #expect(!SensorRole.illuminance.requiresCalibration)
    }

    @Test("Saved baselines are loaded into the calibration fields")
    func loadsSavedBaselines() {
        let link = SensorLink(
            id: UUID(),
            haEntityID: "sensor.pumpkins_soil_moisture",
            role: .soilMoisture,
            dryBaseline: 70.88,
            wetBaseline: 73.97,
            createdAt: Date()
        )

        let draft = SensorCalibrationDraft(link: link)

        #expect(draft.dry == "70.88")
        #expect(draft.wet == "73.97")
        #expect(draft.proposed == SensorCalibration(dryBaseline: 70.88, wetBaseline: 73.97))
    }

    @Test("Missing baselines stay visibly unset")
    func leavesMissingBaselinesEmpty() {
        let link = SensorLink(
            id: UUID(),
            haEntityID: "sensor.new_soil_probe",
            role: .soilMoisture,
            dryBaseline: 42,
            createdAt: Date()
        )

        let draft = SensorCalibrationDraft(link: link)

        #expect(draft.dry == "42")
        #expect(draft.wet.isEmpty)
        #expect(draft.proposed == nil)
    }
}
