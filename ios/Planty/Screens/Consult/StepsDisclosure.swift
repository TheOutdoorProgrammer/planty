import SwiftUI

/// What the model did to reach an answer, collapsed by default. An answer that
/// ran a command or read a page is checkable; one that did neither is the model
/// talking from memory, and those deserve different amounts of trust.
struct StepsDisclosure: View {
    let steps: [AnswerStep]
    @State private var open = false

    var body: some View {
        if !steps.isEmpty {
            VStack(alignment: .leading, spacing: 8) {
                Button {
                    withAnimation(.snappy) { open.toggle() }
                } label: {
                    HStack(spacing: 6) {
                        Image(systemName: open ? "chevron.down" : "chevron.right")
                            .font(.caption2.weight(.semibold))
                        Text(summary)
                            .font(.caption)
                        if refusals > 0 {
                            Image(systemName: "exclamationmark.triangle.fill")
                                .font(.caption2)
                                .foregroundStyle(PlantyColor.orange)
                        }
                        Spacer()
                    }
                    .foregroundStyle(PlantyColor.secondaryText)
                    // Caption text is about sixteen points tall, which is not a
                    // target anybody can hit. Forty-four is Apple's minimum.
                    .frame(minHeight: 44)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityLabel(open ? "Hide what Planty did" : "Show what Planty did")

                if open {
                    VStack(alignment: .leading, spacing: 10) {
                        ForEach(steps) { StepRow(step: $0) }
                    }
                    .padding(.leading, 4)
                    .transition(.opacity.combined(with: .move(edge: .top)))
                }
            }
        }
    }

    private var refusals: Int { steps.filter(\.refused).count }

    /// Counts actions rather than every step, since "3 thoughts" is not a
    /// thing anybody wants to be told.
    private var summary: String {
        let actions = steps.filter { $0.kind == .action }.count
        switch actions {
        case 0: return "How Planty got there"
        case 1: return "1 thing Planty did"
        default: return "\(actions) things Planty did"
        }
    }
}

private struct StepRow: View {
    let step: AnswerStep
    @State private var open = false

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Button {
                withAnimation(.snappy) { open.toggle() }
            } label: {
                HStack(alignment: .top, spacing: 8) {
                    Image(systemName: step.icon)
                        .font(.caption)
                        .foregroundStyle(step.refused ? PlantyColor.orange : PlantyColor.purple)
                        .frame(width: 16)

                    VStack(alignment: .leading, spacing: 2) {
                        Text(step.headline)
                            .font(.caption.weight(.medium))
                            .foregroundStyle(PlantyColor.foreground)
                        if let subtitle = step.subtitle {
                            Text(subtitle)
                                .font(.caption2.monospaced())
                                .foregroundStyle(PlantyColor.secondaryText)
                                .lineLimit(open ? nil : 2)
                        }
                    }
                    Spacer(minLength: 0)
                }
                .frame(minHeight: 44)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .disabled(step.body == nil)

            if open, let body = step.body {
                Text(body)
                    .font(.caption2.monospaced())
                    .foregroundStyle(PlantyColor.secondaryText)
                    .textSelection(.enabled)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(10)
                    .background(
                        PlantyColor.surface,
                        in: RoundedRectangle(cornerRadius: 10, style: .continuous)
                    )
                    .padding(.leading, 24)
            }
        }
    }
}
