import SwiftUI

struct PlantSensorSection: View {
    let series: [SensorSeries]

    private var soilSeries: [SensorSeries] {
        series.filter { $0.link.role == .soilMoisture || $0.link.role == .ambientTemp }
    }

    var body: some View {
        if !soilSeries.isEmpty {
            VStack(alignment: .leading, spacing: 14) {
                HStack(spacing: 9) {
                    Image(systemName: "drop.degreesign.fill")
                        .font(.title3.weight(.semibold))
                        .foregroundStyle(PlantyColor.cyan)
                    Text("In the pot")
                        .font(.headline.weight(.bold))
                    Spacer(minLength: 8)
                    Text("LATEST")
                        .font(.caption2.weight(.black))
                        .tracking(0.8)
                        .foregroundStyle(PlantyColor.cyan)
                        .padding(.horizontal, 8)
                        .padding(.vertical, 5)
                        .background(PlantyColor.cyan.opacity(0.12), in: Capsule())
                }

                ForEach(soilSeries) { sensor in
                    PlantSensorReadingRow(series: sensor)
                    if sensor.id != soilSeries.last?.id {
                        Divider().overlay(PlantyColor.quietDecoration)
                    }
                }

                if soilSeries.contains(where: { $0.link.role == .soilMoisture && !$0.link.isCalibrated }) {
                    Label(
                        "Moisture is the probe's raw reading. "
                            + "Calibrate it before Planty uses it to decide when to water.",
                        systemImage: "scope"
                    )
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .plantyCard(border: PlantyColor.cyan.opacity(0.3))
        }
    }
}

private struct PlantSensorReadingRow: View {
    let series: SensorSeries

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 12) {
            VStack(alignment: .leading, spacing: 3) {
                Text(series.link.role.label)
                    .font(.subheadline.weight(.semibold))
                Text(detail)
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            Spacer(minLength: 12)
            Text(value)
                .font(.title3.monospacedDigit().weight(.bold))
                .foregroundStyle(PlantyColor.foreground)
                .multilineTextAlignment(.trailing)
        }
        .accessibilityElement(children: .combine)
    }

    private var value: String {
        guard let reading = series.latest else { return "No reading" }
        if series.link.role == .soilMoisture,
           let fraction = series.link.fraction(of: reading.value) {
            return fraction.formatted(.percent.precision(.fractionLength(0)))
        }
        return rawValue(reading)
    }

    private var detail: String {
        guard let reading = series.latest else { return "Waiting for the first sample" }
        let when = RelativeAge.dayAndTime(reading.takenAt, now: Date())
        if series.link.role == .soilMoisture, series.link.isCalibrated {
            return "Probe-relative moisture, \(when)"
        }
        if series.link.role == .soilMoisture {
            return "Raw probe reading, \(when)"
        }
        return when
    }

    private func rawValue(_ reading: Reading) -> String {
        let number = reading.value.formatted(.number.precision(.fractionLength(0...1)))
        guard let unit = reading.unit?.trimmingCharacters(in: .whitespacesAndNewlines), !unit.isEmpty else {
            return number
        }
        return ["%", "°F", "°C"].contains(unit) ? number + unit : number + " " + unit
    }
}
