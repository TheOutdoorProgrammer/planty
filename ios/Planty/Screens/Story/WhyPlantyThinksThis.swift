import Charts
import SwiftUI

/// The only place in the app a sensor chart is allowed to appear. Collapsed by
/// default: a beginner should never read a graph to decide whether to water.
struct WhyPlantyThinksThis: View {
    let series: [SensorSeries]
    let verdict: Verdict?

    @State private var isExpanded = false

    var body: some View {
        if !series.isEmpty || verdict != nil {
            VStack(alignment: .leading, spacing: 0) {
                PlantyDisclosureHeader(
                    title: "Why Planty thinks this",
                    icon: "chart.xyaxis.line",
                    isExpanded: $isExpanded,
                    color: PlantyColor.cyan
                )
                if isExpanded {
                    VStack(alignment: .leading, spacing: 18) {
                        if let verdict {
                            VerdictEvidenceBlock(verdict: verdict)
                        }
                        ForEach(series) { one in
                            SensorChartCard(series: one)
                        }
                    }
                    .padding(.top, 12)
                }
            }
            .plantyCard(border: PlantyColor.cyan.opacity(0.3))
        }
    }
}

struct VerdictEvidenceBlock: View {
    let verdict: Verdict

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(verdict.reasoning)
                .font(.footnote)
                .foregroundStyle(PlantyColor.secondaryText)
            if let summary = verdict.evidence.sensorSummary {
                Text(summary)
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            if let citation = verdict.evidence.citationLine {
                Text(citation)
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            if let model = verdict.evidence.modelVersion {
                Text("Model \(model)")
                    .font(.caption2)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct SensorChartCard: View {
    let series: SensorSeries

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Text(series.link.role.label)
                    .font(.subheadline.weight(.semibold))
                Spacer(minLength: 8)
                Text(series.link.haEntityID)
                    .font(.caption2)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            if series.link.role.requiresCalibration && !series.link.isCalibrated {
                Label(
                    "Not calibrated. These numbers cannot drive a decision.",
                    systemImage: "exclamationmark.triangle"
                )
                .font(.caption)
                .foregroundStyle(PlantyColor.yellow)
            }

            chart
                .frame(height: 140)
                .accessibilityLabel(chartDescription)

            if let latest = series.latest {
                Text(latestLine(latest))
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var chart: some View {
        Chart(series.readings) { reading in
            LineMark(
                x: .value("When", reading.takenAt),
                y: .value(series.link.role.label, reading.value)
            )
            .foregroundStyle(PlantyColor.cyan)
            .interpolationMethod(.monotone)
        }
        .chartYScale(domain: series.span ?? 0...1)
        .chartXAxis { AxisMarks(values: .automatic(desiredCount: 3)) }
        .chartYAxis { AxisMarks(position: .leading) }
    }

    private func latestLine(_ reading: Reading) -> String {
        let value = reading.value.formatted(.number.precision(.fractionLength(0...1)))
        let unit = reading.unit ?? ""
        let when = RelativeAge.dayAndTime(reading.takenAt, now: Date())
        if let fraction = series.link.fraction(of: reading.value) {
            let percent = fraction.formatted(.percent.precision(.fractionLength(0)))
            return "Latest \(value)\(unit), \(percent) between this probe's dry and wet. \(when)."
        }
        return "Latest \(value)\(unit), \(when)."
    }

    private var chartDescription: String {
        guard let latest = series.latest else { return "No readings." }
        return """
            \(series.link.role.label) over \(series.readings.count) readings, \
            latest \(latest.value.formatted()).
            """
    }
}
