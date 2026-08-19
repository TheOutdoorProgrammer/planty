import SwiftUI

struct MessageRow: View {
    let message: DiagnosisMessage
    let plant: Plant
    /// Which offered actions are already filed, so the button says so instead
    /// of quietly recording a second copy.
    var taken: Set<String> = []
    let perform: (DiagnosisOfferedAction) -> Void

    var body: some View {
        switch message.speaker {
        case .user:
            userBubble
        case .planty:
            if let turn = message.turn {
                TurnCard(turn: turn, plantName: plant.commonName, taken: taken, perform: perform)
            } else {
                plantyBubble
            }
        }
    }

    private var userBubble: some View {
        HStack {
            Spacer(minLength: 40)
            Text(message.text)
                .font(.body)
                .foregroundStyle(PlantyColor.background)
                .padding(.horizontal, 14)
                .padding(.vertical, 10)
                .background(PlantyColor.pink, in: RoundedRectangle(cornerRadius: 18))
        }
        .accessibilityLabel("You said: \(message.text)")
    }

    private var plantyBubble: some View {
        Text(message.text)
            .font(.body)
            .frame(maxWidth: .infinity, alignment: .leading)
            .plantyCard(padding: 14)
    }
}

/// Separates what can be seen, what it probably means, and what to do today.
/// Urgent gets red and an explicit label, and never gets the mascot.
struct TurnCard: View {
    let turn: DiagnosisTurn
    let plantName: String
    /// Which offered actions are already filed, so a second tap says so
    /// instead of quietly recording a duplicate.
    var taken: Set<String> = []
    let perform: (DiagnosisOfferedAction) -> Void

    private var accent: Color {
        switch turn.severity {
        case .urgent: PlantyColor.red
        case .insufficient: PlantyColor.yellow
        case .noConcern: PlantyColor.green
        case .finding, .unknown: PlantyColor.cyan
        }
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            if turn.severity.isUrgent {
                Label("Urgent", systemImage: "exclamationmark.triangle.fill")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(PlantyColor.red)
            }

            Text(turn.observed)
                .font(.headline)

            if let interpretation = turn.interpretation {
                labelled("What it probably means", interpretation)
            }
            if let actionToday = turn.actionToday {
                Text(actionToday)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(accent)
            }
            if let plan = turn.followUpPlan {
                Text(plan)
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            if let citation = turn.citation {
                Text(citation)
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            ForEach(turn.offeredActions) { action in
                let done = taken.contains(action.title)
                Button(done ? "\(action.title) — recorded" : action.title) {
                    perform(action)
                }
                .buttonStyle(PrimaryButtonStyle(color: done ? PlantyColor.quietDecoration : accent))
                .disabled(done)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: accent.opacity(0.45))
        .accessibilityElement(children: .contain)
        .accessibilityLabel("Planty's reading of \(plantName)")
    }

    private func labelled(_ heading: String, _ text: String) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Eyebrow(text: heading, color: PlantyColor.secondaryText)
            Text(text)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
        }
    }
}
