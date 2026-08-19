import SwiftUI

extension ToxicityRating {
    /// Unknown is purple, off the green-to-red severity ramp entirely, so it
    /// cannot read as one notch gentler than safe.
    var color: Color {
        switch self {
        case .safe: PlantyColor.green
        case .unknown: PlantyColor.purple
        case .mild: PlantyColor.yellow
        case .moderate: PlantyColor.orange
        case .severe: PlantyColor.red
        }
    }
}

/// Who this plant is dangerous to. An unchecked rating is never a calm chip
/// beside a green one: it is dashed, purple, and says "Not checked" in words.
struct ToxicityCard<Ask: View>: View {
    let toxicity: Toxicity
    let plantName: String

    /// Somewhere the user can actually get an answer, which is the only useful
    /// thing to offer about a plant nobody has looked up.
    @ViewBuilder var ask: Ask

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            Eyebrow(text: "Toxicity", color: toxicity.worst.color)

            if toxicity.isUnchecked {
                unchecked
            } else {
                headline
                if let sentence = toxicity.divergenceSentence {
                    divergence(sentence)
                }
                chips
                if let firstAid = toxicity.firstAid {
                    firstAidCard(firstAid)
                }
                details
                provenance
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: toxicity.worst.color.opacity(0.5))
    }

    private var headline: some View {
        VStack(alignment: .leading, spacing: 6) {
            Label(toxicity.headline, systemImage: toxicity.worst.symbol)
                .font(.title3.weight(.bold))
                .foregroundStyle(toxicity.worst.color)
            if toxicity.isCheckedButUnresolved {
                Text("""
                    Planty looked and could not establish this. Treat it as \
                    unknown, not as safe.
                    """)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
    }

    /// Said in words rather than left for somebody to spot by comparing chips.
    /// Most houseplants read the same for everyone; the ones that do not kill.
    private func divergence(_ sentence: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Label("Not the same risk for everyone", systemImage: "arrow.triangle.branch")
                .font(.subheadline.weight(.bold))
                .foregroundStyle(PlantyColor.foreground)
            Text(sentence)
                .font(.headline)
                .foregroundStyle(toxicity.worst.color)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(12)
        .background(
            toxicity.worst.color.opacity(0.14),
            in: RoundedRectangle(cornerRadius: 16, style: .continuous)
        )
        .accessibilityElement(children: .combine)
    }

    private var chips: some View {
        HStack(alignment: .top, spacing: 8) {
            ForEach(toxicity.ratings, id: \.audience) { entry in
                ToxicityChip(audience: entry.audience, rating: entry.rating)
            }
        }
    }

    /// Loudest thing after the chips on purpose: it only exists for the cases
    /// where the obvious response is the wrong one.
    private func firstAidCard(_ text: String) -> some View {
        VStack(alignment: .leading, spacing: 8) {
            Label("First aid", systemImage: "cross.case.fill")
                .font(.caption.weight(.bold))
                .foregroundStyle(PlantyColor.red)
            Text(text)
                .font(.title3.weight(.semibold))
                .foregroundStyle(PlantyColor.foreground)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(14)
        .background(
            PlantyColor.red.opacity(0.16),
            in: RoundedRectangle(cornerRadius: 18, style: .continuous)
        )
        .overlay {
            RoundedRectangle(cornerRadius: 18, style: .continuous)
                .stroke(PlantyColor.red.opacity(0.6), lineWidth: 1)
        }
        .accessibilityElement(children: .combine)
    }

    @ViewBuilder
    private var details: some View {
        VStack(alignment: .leading, spacing: 10) {
            if let principle = toxicity.principle {
                labelled("What is in it", principle)
            }
            if let signs = toxicity.signs {
                labelled("What it does", signs)
            }
            if !toxicity.parts.isEmpty {
                labelled("Parts that are toxic", toxicity.parts.joined(separator: ", "))
            }
            if !toxicity.routes.isEmpty {
                labelled("Routes of exposure", toxicity.routes.joined(separator: ", "))
            }
            if let notes = toxicity.notes {
                labelled("Worth knowing", notes)
            }
        }
    }

    private var provenance: some View {
        VStack(alignment: .leading, spacing: 4) {
            if let identifiedAs = toxicity.identifiedAs {
                Text("Graded as \(identifiedAs).")
            }
            if toxicity.isDerived {
                Text("""
                    Planty's own grading, not the source's. The source publishes \
                    only toxic or not, so anything finer than that was inferred.
                    """)
            }
            if let source = toxicity.source {
                Text("Source: \(source)")
            }
            if let checked = toxicity.checkedLine() {
                Text(checked)
            }
        }
        .font(.caption)
        .foregroundStyle(PlantyColor.secondaryText)
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var unchecked: some View {
        VStack(alignment: .leading, spacing: 10) {
            Label("Nobody has checked this", systemImage: "questionmark.circle")
                .font(.title3.weight(.bold))
                .foregroundStyle(PlantyColor.purple)
            Text("""
                Planty holds no toxicity record for \(plantName). That is not \
                the same as safe: it means nothing has been looked up yet.
                """)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
            ask
        }
    }

    private func labelled(_ heading: String, _ text: String) -> some View {
        VStack(alignment: .leading, spacing: 3) {
            Eyebrow(text: heading, color: PlantyColor.secondaryText)
            Text(text)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

/// Words and an icon, never colour alone. Unchecked gets a dashed outline and
/// no fill, so it cannot be mistaken for one of the settled ratings.
struct ToxicityChip: View {
    let audience: ToxicityAudience
    let rating: ToxicityRating

    var body: some View {
        VStack(spacing: 6) {
            Label(audience.label, systemImage: audience.symbol)
                .font(.caption.weight(.semibold))
                .foregroundStyle(PlantyColor.secondaryText)
            Text(rating.label)
                .font(.footnote.weight(.bold))
                .foregroundStyle(rating.color)
                .multilineTextAlignment(.center)
            Image(systemName: rating.symbol)
                .font(.footnote.weight(.bold))
                .foregroundStyle(rating.color)
                .accessibilityHidden(true)
        }
        .frame(maxWidth: .infinity, minHeight: 88)
        .padding(.vertical, 10)
        .padding(.horizontal, 6)
        .background {
            if rating.isChecked {
                RoundedRectangle(cornerRadius: 16, style: .continuous)
                    .fill(rating.color.opacity(rating == .severe ? 0.28 : 0.16))
            }
        }
        .overlay {
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .strokeBorder(rating.color.opacity(rating.isChecked ? 0.55 : 0.95), style: outline)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel("\(audience.label). \(rating.sentence(for: audience))")
    }

    private var outline: StrokeStyle {
        guard rating.isChecked else { return StrokeStyle(lineWidth: 1.5, dash: [5, 4]) }
        return StrokeStyle(lineWidth: rating == .severe ? 2 : 1)
    }
}
