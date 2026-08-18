import SwiftUI

/// Ranked candidates, never one confident-looking answer. A wrong species
/// stated firmly is worse than three honest options.
struct IdentificationView: View {
    let store: IdentificationStore
    let onPick: (IdentificationCandidate) -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            switch store.stage {
            case .idle:
                EmptyView()
            case .working:
                HStack(spacing: 10) {
                    ProgressView().tint(PlantyColor.green)
                    Text("Looking at the plant…")
                        .font(.subheadline)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            case .done(let outcome):
                results(for: outcome)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    @ViewBuilder
    private func results(for outcome: IdentificationOutcome) -> some View {
        switch outcome {
        case .notAPlant(let coarse):
            notice(
                "That does not look like a plant.",
                detail: coarse.isEmpty
                    ? "Nothing recognisable in the frame."
                    : "It looks more like \(coarse.prefix(2).map(\.readable).joined(separator: " or ")).",
                symbol: "questionmark.circle.fill",
                tint: PlantyColor.yellow
            )

        case .coarseOnly(let coarse, let reason):
            VStack(alignment: .leading, spacing: 12) {
                notice(
                    "No species yet.",
                    detail: reason,
                    symbol: "wifi.exclamationmark",
                    tint: PlantyColor.yellow
                )
                if !coarse.isEmpty {
                    labels(coarse, title: "What the phone could tell on its own")
                }
            }

        case .identified(let result):
            VStack(alignment: .leading, spacing: 12) {
                Text("Best guesses")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(PlantyColor.foreground)

                ForEach(result.candidates) { candidate in
                    Button { onPick(candidate) } label: { row(candidate) }
                        .buttonStyle(.plain)
                }

                if result.metadata.hasLocation || result.metadata.capturedAt != nil {
                    Text(evidence(result.metadata))
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            }
        }
    }

    private func row(_ candidate: IdentificationCandidate) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 10) {
            VStack(alignment: .leading, spacing: 2) {
                Text(candidate.commonName)
                    .font(.body.weight(.semibold))
                    .foregroundStyle(PlantyColor.foreground)
                if let scientific = candidate.scientificName {
                    Text(scientific)
                        .font(.caption.italic())
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            }
            Spacer()
            Text(candidate.strength.label)
                .font(.caption.weight(.semibold))
                .padding(.horizontal, 8)
                .padding(.vertical, 4)
                .background(tint(for: candidate.strength).opacity(0.18), in: Capsule())
                .foregroundStyle(tint(for: candidate.strength))
        }
        .padding(12)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(PlantyColor.surface, in: RoundedRectangle(cornerRadius: 14, style: .continuous))
        .contentShape(Rectangle())
    }

    private func labels(_ coarse: [CoarseLabel], title: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title)
                .font(.caption.weight(.semibold))
                .foregroundStyle(PlantyColor.secondaryText)
            Text(coarse.map(\.readable).joined(separator: " · "))
                .font(.caption)
                .foregroundStyle(PlantyColor.secondaryText)
        }
    }

    private func notice(_ title: String, detail: String, symbol: String, tint: Color) -> some View {
        HStack(alignment: .top, spacing: 10) {
            Image(systemName: symbol).foregroundStyle(tint)
            VStack(alignment: .leading, spacing: 3) {
                Text(title).font(.subheadline.weight(.semibold))
                Text(detail).font(.caption).foregroundStyle(PlantyColor.secondaryText)
            }
        }
    }

    /// Says what narrowed the answer, because "we used where and when" is the
    /// difference between a guess and a reasoned one.
    private func evidence(_ metadata: CaptureMetadata) -> String {
        var parts: [String] = []
        if let capturedAt = metadata.capturedAt {
            parts.append("taken \(capturedAt.formatted(.dateTime.month(.abbreviated).year()))")
        }
        if metadata.hasLocation { parts.append("with a location") }
        if !metadata.fromOriginal { parts.append("from a copy, so some detail was missing") }
        return "Narrowed using the photo's own details: " + parts.joined(separator: ", ") + "."
    }

    private func tint(for strength: IdentificationCandidate.Strength) -> Color {
        switch strength {
        case .likely: PlantyColor.green
        case .possible: PlantyColor.yellow
        case .weak: PlantyColor.secondaryText
        }
    }
}
