import SwiftUI

/// A Today card is now a visual summary and a single tap target. All choices
/// live on the dedicated action screen so the card never makes camera capture
/// or postponement feel mandatory.
struct CareCard: View {
    let entry: DigestEntry

    private var state: CareState { CareState.from(action: entry.verdict.action) }
    private var accent: Color { state.color }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            PlantPhotoView(plant: entry.plant, height: 156, opensFullScreen: false)
                .allowsHitTesting(false)

            header
            name
            Text(entry.verdict.action.instruction)
                .font(.headline)
                .foregroundStyle(PlantyColor.foreground)
            reasoning

            HStack(spacing: 8) {
                Text("Tap for options")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(PlantyColor.secondaryText)
                Spacer(minLength: 0)
                Image(systemName: "chevron.right")
                    .font(.footnote.weight(.bold))
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: accent.opacity(entry.plant.isFriends ? 0.0 : 0.45))
        .overlay {
            if entry.plant.isFriends {
                RoundedRectangle(cornerRadius: 24, style: .continuous)
                    .stroke(PlantyColor.purple.opacity(0.6), lineWidth: 1.5)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityLabel(accessibilitySummary)
        .accessibilityHint("Opens actions for \(entry.plant.commonName)")
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

    private var accessibilitySummary: String {
        var parts = [entry.plant.commonName, state.label, entry.verdict.action.instruction]
        if entry.plant.isFriends {
            parts.insert(entry.plant.ownershipAccessibilityLabel, at: 0)
        }
        return parts.joined(separator: ". ")
    }
}

/// The sensor evidence, folded away on the action screen. Charts are never the
/// default view.
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
