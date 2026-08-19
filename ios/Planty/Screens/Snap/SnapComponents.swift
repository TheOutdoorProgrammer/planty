import PhotosUI
import SwiftUI

/// Large, tappable, and prefilled from the care card or the last plant seen.
struct PlantChip: View {
    let plant: Plant?
    let pick: () -> Void

    var body: some View {
        Button(action: pick) {
            HStack(spacing: 10) {
                Image(systemName: "leaf.fill")
                    .foregroundStyle(PlantyColor.green)
                VStack(alignment: .leading, spacing: 2) {
                    Text(plant?.commonName ?? "Pick a plant")
                        .font(.headline)
                    if let plant {
                        Text(subtitle(for: plant))
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                    } else {
                        Text("You can shoot first and choose after.")
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                }
                Spacer(minLength: 8)
                Image(systemName: "chevron.down")
                    .font(.footnote.weight(.bold))
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            .frame(maxWidth: .infinity, minHeight: 56, alignment: .leading)
            .padding(.horizontal, 16)
            .padding(.vertical, 10)
            .background(PlantyColor.surface, in: Capsule())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(plant.map { "Selected plant, \($0.commonName)" } ?? "No plant selected")
        .accessibilityHint("Opens the plant picker")
    }

    private func subtitle(for plant: Plant) -> String {
        [plant.isFriends ? plant.ownershipLabel : nil, plant.location]
            .compactMap { $0 }
            .filter { !$0.isEmpty }
            .joined(separator: " · ")
    }
}

struct CameraStage: View {
    let camera: CameraController
    @Binding var photoItem: PhotosPickerItem?

    /// Overridden where the shot is not a whole-plant portrait: a chat asking
    /// for the underside of a leaf must not tell you to frame the whole plant.
    var guidance = "Fit the whole plant in the frame."
    var footnote = "One photo is enough."
    let shutter: () -> Void

    @ScaledMetric(relativeTo: .largeTitle) private var shutterSize: CGFloat = 74

    var body: some View {
        VStack(spacing: 16) {
            preview
            Text(guidance)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)

            HStack(spacing: 24) {
                PhotosPicker(selection: $photoItem, matching: .images) {
                    Image(systemName: "photo.on.rectangle")
                        .font(.title3)
                        .frame(width: 52, height: 52)
                        .background(PlantyColor.surface, in: Circle())
                }
                .accessibilityLabel("Choose from Photos")

                Button(action: shutter) {
                    Circle()
                        .fill(PlantyColor.pink)
                        .frame(width: shutterSize, height: shutterSize)
                        .overlay {
                            Circle().stroke(PlantyColor.foreground.opacity(0.8), lineWidth: 3)
                        }
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Take plant photo")

                Color.clear.frame(width: 52, height: 52)
                    .accessibilityHidden(true)
            }

            Text(footnote)
                .font(.footnote)
                .foregroundStyle(PlantyColor.secondaryText)
        }
    }

    /// Double tap is deliberately not wired to the shutter: duplicate records
    /// are worse than a missed tap.
    private var preview: some View {
        ZStack {
            if camera.availability == .ready {
                CameraPreview(session: camera.session)
            } else {
                LinearGradient(
                    colors: [Color(hex: 0x394B43), Color(hex: 0x26332F)],
                    startPoint: .top,
                    endPoint: .bottom
                )
                ProgressView()
            }
        }
        .frame(height: 380)
        .clipShape(RoundedRectangle(cornerRadius: 28, style: .continuous))
        .accessibilityHidden(true)
    }
}

struct CameraPermissionCard: View {
    @Binding var photoItem: PhotosPickerItem?

    var body: some View {
        StateMessage(
            title: "Planty needs the camera for the useful bit.",
            message: """
                Photos let Planty compare changes that soil and room sensors \
                cannot see.
                """,
            accent: PlantyColor.pink,
            icon: "camera.fill"
        ) {
            Button("Allow camera access") {
                guard let url = URL(string: UIApplication.openSettingsURLString) else { return }
                UIApplication.shared.open(url)
            }
            .buttonStyle(PrimaryButtonStyle())

            PhotosPicker(selection: $photoItem, matching: .images) {
                Text("Choose from Photos")
                    .font(.headline)
                    .foregroundStyle(PlantyColor.foreground)
                    .frame(maxWidth: .infinity, minHeight: 50)
                    .background(PlantyColor.surface, in: Capsule())
            }
        }
    }
}

/// Every simulator lands here, and so does any device with no usable camera.
struct NoCameraCard: View {
    @Binding var photoItem: PhotosPickerItem?

    var body: some View {
        StateMessage(
            title: "No camera on this device.",
            message: "Pick an existing photo instead. Everything else works the same.",
            accent: PlantyColor.cyan,
            icon: "camera.badge.ellipsis"
        ) {
            PhotosPicker(selection: $photoItem, matching: .images) {
                Text("Choose from Photos")
                    .font(.headline)
                    .foregroundStyle(PlantyColor.background)
                    .frame(maxWidth: .infinity, minHeight: 52)
                    .background(PlantyColor.cyan, in: Capsule())
            }
        }
    }
}

struct SaveFailedCard: View {
    let error: PlantyError
    let retry: () -> Void
    let discard: () -> Void

    var body: some View {
        StateMessage(
            title: "That photo did not save.",
            message: """
                It is still on this screen. Try again before closing so the \
                observation is not lost.
                """,
            accent: PlantyColor.orange,
            icon: "exclamationmark.arrow.trianglehead.2.clockwise.rotate.90"
        ) {
            if let detail = error.errorDescription {
                Text(detail)
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            Button("Try saving again", action: retry)
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
            Button("Discard photo", action: discard)
                .buttonStyle(SecondaryButtonStyle())
        }
    }
}
