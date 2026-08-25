import SwiftUI

/// The first photo stays put and the scrubber moves the second through time,
/// which is the comparison somebody actually wants: is it better than when I
/// got it, not is it better than last Tuesday.
struct PhotoComparisonScreen: View {
    let plant: Plant
    let comparison: PhotoComparison

    @State private var index: Int
    @State private var mode = PhotoComparisonMode.overlay
    @State private var overlayOpacity = 0.5
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize

    init(plant: Plant, comparison: PhotoComparison) {
        self.plant = plant
        self.comparison = comparison
        _index = State(initialValue: comparison.lastIndex)
    }

    var body: some View {
        ScrollView {
            VStack(spacing: 20) {
                Picker("Comparison style", selection: $mode) {
                    ForEach(PhotoComparisonMode.allCases, id: \.self) { option in
                        Text(option.label).tag(option)
                    }
                }
                .pickerStyle(.segmented)

                if mode == .overlay {
                    overlay
                } else if dynamicTypeSize.isAccessibilitySize {
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

    private var overlay: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text("First photo")
                Spacer()
                Text("Review at \(overlayOpacity.formatted(.percent.precision(.fractionLength(0))))")
            }
            .font(.caption.weight(.semibold))
            .foregroundStyle(PlantyColor.secondaryText)

            ZStack {
                PlantPhotoView(plant: plant, photo: comparison.earliest, height: 360)
                PlantPhotoView(plant: plant, photo: comparison.photo(at: index), height: 360)
                    .opacity(overlayOpacity)
            }
            .allowsHitTesting(false)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(overlayAccessibilityLabel)

            Slider(value: $overlayOpacity, in: 0...1)
                .tint(PlantyColor.cyan)
                .accessibilityLabel("Review photo opacity")
                .accessibilityValue(overlayOpacity.formatted(.percent.precision(.fractionLength(0))))
        }
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

    private var overlayAccessibilityLabel: String {
        let first = comparison.earliest?.accessibilityDescription(plantName: plant.commonName)
            ?? "First photo unavailable"
        let review = comparison.photo(at: index)?.accessibilityDescription(plantName: plant.commonName)
            ?? "Review photo unavailable"
        return "Overlay comparison. \(first). \(review)."
    }
}

private enum PhotoComparisonMode: CaseIterable {
    case overlay
    case sideBySide

    var label: String {
        switch self {
        case .overlay: "Overlay"
        case .sideBySide: "Side by side"
        }
    }
}
