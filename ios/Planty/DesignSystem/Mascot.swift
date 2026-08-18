import SwiftUI

/// The seal. It appears in calm, empty and success states only, never beside a
/// severe alert, where a grinning mascot would read as contempt.
struct PlantyMascot: View {
    var wobbles = true

    @ScaledMetric(relativeTo: .largeTitle) private var side: CGFloat = 168
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var tilt: Double = 0

    var body: some View {
        Image("PlantyMascot")
            .resizable()
            .scaledToFit()
            .frame(width: side, height: side)
            .rotationEffect(.degrees(tilt), anchor: .bottom)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(Self.description)
            .task {
                guard wobbles, !reduceMotion else { return }
                withAnimation(.easeInOut(duration: 0.9)) { tilt = -3 }
                try? await Task.sleep(for: .seconds(0.9))
                withAnimation(.easeInOut(duration: 0.9)) { tilt = 0 }
            }
    }

    static let description = """
        Planty, a delighted lavender seal cheerfully overwatering a potted plant
        """
}

/// Draws a plant photo, or an honest stand-in when the bytes are not here.
/// Fetching by storage key has no documented URL yet; see ios/README.md.
struct PlantPhotoView: View {
    let plant: Plant
    var photo: Photo?
    var localJPEG: Data?
    var height: CGFloat = 220

    var body: some View {
        Group {
            if let localJPEG, let image = UIImage(data: localJPEG) {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFill()
            } else {
                placeholder
            }
        }
        .frame(maxWidth: .infinity)
        .frame(height: height)
        .clipShape(RoundedRectangle(cornerRadius: 24, style: .continuous))
        .overlay {
            RoundedRectangle(cornerRadius: 24, style: .continuous)
                .stroke(PlantyColor.foreground.opacity(0.12), lineWidth: 1)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(label)
    }

    /// A thumbnail and a hero need different furniture: the caption pill and a
    /// big glyph turn to mush at list-row size.
    private var isThumbnail: Bool { height < 110 }

    private var placeholder: some View {
        ZStack {
            LinearGradient(
                colors: [Color(hex: 0x35534B), Color(hex: 0x1F2D2A), PlantyColor.surface],
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            )
            Image(systemName: "leaf.fill")
                .font(isThumbnail ? .title3 : .system(size: 64, weight: .light))
                .foregroundStyle(PlantyColor.green.opacity(0.85))
                .accessibilityHidden(true)

            if !isThumbnail {
                VStack {
                    Spacer()
                    HStack {
                        Text(caption)
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(PlantyColor.foreground)
                            .padding(.horizontal, 10)
                            .padding(.vertical, 7)
                            .background(PlantyColor.background.opacity(0.78), in: Capsule())
                        Spacer()
                    }
                    .padding(14)
                }
            }
        }
    }

    private var caption: String {
        guard let photo else { return "No photo yet" }
        return photo.takenAt.formatted(.dateTime.month(.abbreviated).day())
    }

    private var label: String {
        guard let photo else {
            return "No photo of \(plant.commonName) yet."
        }
        return photo.accessibilityDescription(plantName: plant.commonName)
    }
}
