import SwiftUI

/// A score is evidence, not a diagnosis or lifecycle control. Words and the
/// numeric value carry the meaning; color only helps scanning.
struct PlantHealthBar: View {
    let event: HealthEvent?
    var compact = false

    private var presentation: HealthPresentation {
        HealthPresentation(score: event?.score)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: compact ? 4 : 8) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text(event == nil ? "Health unknown" : "Health evidence")
                    .font(compact ? .caption.weight(.semibold) : .subheadline.weight(.semibold))
                Spacer(minLength: 8)
                if let score = event?.score {
                    Text("\(number(score)) / 100")
                        .font((compact ? Font.caption : Font.subheadline).monospacedDigit().weight(.bold))
                }
            }

            if let score = event?.score {
                ProgressView(value: score, total: 100)
                    .tint(color(for: score))
            } else if !compact {
                Text("No baseline has been recorded yet.")
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            if event?.score == 0, !compact {
                Text("0 means the evidence supports dead or unrecoverable. Archiving is a separate confirmation.")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(PlantyColor.red)
            } else if event?.score == 100, !compact {
                Text("100 means no known or visible health deficit.")
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(presentation.accessibilityDescription)
    }

    private func number(_ value: Double) -> String {
        value.formatted(.number.precision(.fractionLength(0...1)))
    }

    private func color(for score: Double) -> Color {
        switch score {
        case 75...: PlantyColor.green
        case 40..<75: PlantyColor.yellow
        case 0: PlantyColor.red
        case 0..<40: PlantyColor.orange
        default: PlantyColor.red
        }
    }
}
