import SwiftUI

/// Leads with the physical action, then the reason. No score, no raw moisture
/// value, and no red unless the verdict is genuine evidence of harm.
struct CareCard: View {
    let entry: DigestEntry
    let imHere: () -> Void
    let notNow: () -> Void

    private var state: CareState { CareState.from(action: entry.verdict.action) }
    private var accent: Color { state.color }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            header
            name
            Text(entry.verdict.action.instruction)
                .font(.headline)
                .foregroundStyle(PlantyColor.foreground)
            reasoning
            evidence
            buttons
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: accent.opacity(entry.plant.isFriends ? 0.0 : 0.45))
        .overlay {
            if entry.plant.isFriends {
                RoundedRectangle(cornerRadius: 24, style: .continuous)
                    .stroke(PlantyColor.purple.opacity(0.6), lineWidth: 1.5)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(accessibilitySummary)
    }

    private var header: some View {
        ViewThatFits(in: .horizontal) {
            HStack(spacing: 10) {
                OwnershipBadge(plant: entry.plant)
                StatusPill(state: state)
                Spacer(minLength: 0)
            }
            VStack(alignment: .leading, spacing: 8) {
                OwnershipBadge(plant: entry.plant)
                StatusPill(state: state)
            }
        }
    }

    private var name: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(entry.plant.commonName)
                .font(.title2.weight(.bold))
            Text(subtitle)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
        }
    }

    private var subtitle: String {
        [entry.plant.displaySpecies, entry.plant.location]
            .compactMap { $0 }
            .filter { !$0.isEmpty }
            .joined(separator: " · ")
    }

    @ViewBuilder
    private var reasoning: some View {
        if !entry.verdict.reasoning.isEmpty {
            Text(entry.verdict.reasoning)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
        }
    }

    @ViewBuilder
    private var evidence: some View {
        if !entry.verdict.evidence.isEmpty {
            DisclosureGroup("Why Planty thinks this") {
                EvidenceDetail(verdict: entry.verdict)
                    .padding(.top, 8)
            }
            .font(.footnote.weight(.semibold))
            .tint(PlantyColor.cyan)
        }
    }

    /// No swipe actions on purpose: accidental completion has real consequences.
    private var buttons: some View {
        ViewThatFits(in: .horizontal) {
            HStack(spacing: 12) {
                imHereButton
                notNowButton
            }
            VStack(spacing: 10) {
                imHereButton
                notNowButton
            }
        }
    }

    private var imHereButton: some View {
        Button("I'm here", action: imHere)
            .buttonStyle(PrimaryButtonStyle(color: accent))
            .accessibilityHint("Opens the camera with \(entry.plant.commonName) selected")
    }

    private var notNowButton: some View {
        Button("Not now", action: notNow)
            .buttonStyle(SecondaryButtonStyle())
            .accessibilityHint("Postpones this card. It is never marked done.")
    }

    private var accessibilitySummary: String {
        var parts = [entry.plant.commonName, state.label, entry.verdict.action.instruction]
        if entry.plant.isFriends {
            parts.insert(entry.plant.ownershipAccessibilityLabel, at: 0)
        }
        return parts.joined(separator: ". ")
    }
}

/// The sensor evidence, folded away. Charts are never the default view.
struct EvidenceDetail: View {
    let verdict: Verdict

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            if let summary = verdict.evidence.sensorSummary, !summary.isEmpty {
                Text(summary)
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            if let citation = verdict.evidence.citationLine {
                Text(citation)
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            Text("Confidence: \(verdict.confidence.formatted(.percent.precision(.fractionLength(0))))")
                .font(.caption)
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}
