import SwiftUI

/// The first photo stays put and the scrubber moves the second through time,
/// which is the comparison somebody actually wants: is it better than when I
/// got it, not is it better than last Tuesday.
struct PhotoComparisonScreen: View {
    let plant: Plant
    let comparison: PhotoComparison

    @State private var index: Int
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    init(plant: Plant, comparison: PhotoComparison) {
        self.plant = plant
        self.comparison = comparison
        _index = State(initialValue: comparison.lastIndex)
    }

    var body: some View {
        ScrollView {
            VStack(spacing: 20) {
                if dynamicTypeSize.isAccessibilitySize {
                    VStack(alignment: .leading, spacing: 16) {
                        pane(comparison.earliest, caption: "First")
                        pane(comparison.photo(at: index), caption: "Then")
                    }
                } else {
                    ViewThatFits(in: .horizontal) {
                        HStack(alignment: .top, spacing: 12) {
                            pane(comparison.earliest, caption: "First")
                            pane(comparison.photo(at: index), caption: "Then")
                        }
                        VStack(alignment: .leading, spacing: 16) {
                            pane(comparison.earliest, caption: "First")
                            pane(comparison.photo(at: index), caption: "Then")
                        }
                    }
                }

                if let span {
                    Text(span)
                        .font(.headline)
                        .foregroundStyle(PlantyColor.foreground)
                }

                scrubber

                if let finding = comparison.photo(at: index)?.visionFindings, !finding.isEmpty {
                    Text(finding)
                        .font(.callout)
                        .foregroundStyle(PlantyColor.secondaryText)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
            .padding(20)
        }
        .plantyPage()
        .navigationTitle("Compare")
        .navigationBarTitleDisplayMode(.inline)
    }

    /// The label sits above its photo: which one this is has to be known before
    /// it is looked at, not explained afterwards.
    private func pane(_ photo: Photo?, caption: String) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(caption)
                .font(.caption.weight(.semibold))
                .foregroundStyle(PlantyColor.foreground)
            PlantPhotoView(plant: plant, photo: photo, height: 200)
            if let taken = photo?.takenAt {
                Text(taken.formatted(.dateTime.month(.abbreviated).day().year()))
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .frame(minWidth: 220)
    }

    @ViewBuilder
    private var scrubber: some View {
        if comparison.lastIndex > 0 {
            Slider(
                value: Binding(
                    get: { Double(index) },
                    set: { index = Int($0.rounded()) }
                ),
                in: 0...Double(comparison.lastIndex),
                step: 1
            )
            .tint(PlantyColor.green)
            .accessibilityLabel("Which photo to compare against the first")
            .accessibilityValue(comparison.photo(at: index).map {
                $0.accessibilityDescription(plantName: plant.commonName)
            } ?? "No photo")
        }
    }

    private var span: String? {
        guard let first = comparison.earliest?.takenAt,
              let chosen = comparison.photo(at: index)?.takenAt
        else { return nil }
        return PhotoComparison.span(between: first, and: chosen)
    }
}
