import SwiftUI

struct PlantSensorSection: View {
    let series: [SensorSeries]
    let proposals: [CalibrationProposal]
    let onResolve: (CalibrationProposal, Bool) async -> PlantyError?

    @State private var resolving: Set<UUID> = []
    @State private var failure: PlantyError?

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

                ForEach(proposals) { proposal in
                    CalibrationProposalCard(
                        proposal: proposal,
                        isWorking: resolving.contains(proposal.id),
                        onResolve: resolve
                    )
                }

                if let failure {
                    SheetErrorRow(headline: "The calibration proposal was not changed.", error: failure)
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

    private func resolve(_ proposal: CalibrationProposal, approve: Bool) async {
        guard !resolving.contains(proposal.id) else { return }
        resolving.insert(proposal.id)
        defer { resolving.remove(proposal.id) }
        failure = await onResolve(proposal, approve)
    }
}

private struct CalibrationProposalCard: View {
    let proposal: CalibrationProposal
    let isWorking: Bool
    let onResolve: (CalibrationProposal, Bool) async -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Label("Planty suggests recalibrating", systemImage: "scope")
                .font(.subheadline.weight(.bold))
                .foregroundStyle(PlantyColor.orange)
            Text(proposal.reason)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
            LabeledContent("Probe now", value: actualValue)
            LabeledContent(
                "Relative now",
                value: percent(proposal.currentRelative) + " → " + percent(proposal.proposedRelative)
            )
            LabeledContent(
                "Dry / wet",
                value: values(proposal.currentDry, proposal.currentWet)
                    + " → " + values(proposal.proposedDry, proposal.proposedWet)
            )
            HStack(spacing: 10) {
                Button("Deny", role: .destructive) {
                    Task { await onResolve(proposal, false) }
                }
                .buttonStyle(.bordered)
                .disabled(isWorking)
                Button("Approve") {
                    Task { await onResolve(proposal, true) }
                }
                .buttonStyle(.borderedProminent)
                .tint(PlantyColor.green)
                .disabled(isWorking)
            }
        }
        .padding(12)
        .background(PlantyColor.orange.opacity(0.08), in: RoundedRectangle(cornerRadius: 14))
    }

    private var actualValue: String {
        let number = proposal.actualValue.formatted(.number.precision(.fractionLength(0...1)))
        guard let unit = proposal.unit?.nilIfBlank else { return number }
        return ["%", "°F", "°C"].contains(unit) ? number + unit : number + " " + unit
    }

    private func percent(_ value: Double) -> String {
        value.formatted(.percent.precision(.fractionLength(0)))
    }

    private func values(_ dry: Double, _ wet: Double) -> String {
        let format = FloatingPointFormatStyle<Double>.number.precision(.fractionLength(0...1))
        return dry.formatted(format) + " / " + wet.formatted(format)
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
            VStack(alignment: .trailing, spacing: 3) {
                Text(value)
                    .font(.title3.monospacedDigit().weight(.bold))
                    .foregroundStyle(PlantyColor.foreground)
                if let probeValue {
                    Text("Probe \(probeValue)")
                        .font(.caption.monospacedDigit())
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            }
            .multilineTextAlignment(.trailing)
        }
        .accessibilityElement(children: .combine)
    }

    private var value: String {
        guard let reading = series.latest else { return "No reading" }
        if series.link.role == .soilMoisture,
           let fraction = series.link.fraction(of: reading.value) {
            return "Relative " + fraction.formatted(.percent.precision(.fractionLength(0)))
        }
        return rawValue(reading)
    }

    private var probeValue: String? {
        guard series.link.role == .soilMoisture,
              series.link.isCalibrated,
              let reading = series.latest
        else { return nil }
        return rawValue(reading)
    }

    private var detail: String {
        guard let reading = series.latest else { return "Waiting for the first sample" }
        let when = RelativeAge.dayAndTime(reading.takenAt, now: Date())
        if series.link.role == .soilMoisture, series.link.isCalibrated {
            return "Calibrated from this probe's dry and wet values, \(when)"
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
