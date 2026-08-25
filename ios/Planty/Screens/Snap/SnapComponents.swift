import PhotosUI
import SwiftUI

struct PlantChip: View {
    let plant: Plant?
    let pick: () -> Void

    var body: some View {
        Button(action: pick) {
            HStack(spacing: 10) {
                Image(systemName: plant == nil ? "leaf.circle" : "leaf.fill")
                    .foregroundStyle(PlantyColor.green)
                VStack(alignment: .leading, spacing: 2) {
                    Text(plant?.commonName ?? "Choose a plant")
                        .font(.headline)
                    Text(plant.map(subtitle) ?? "Optional — you can choose after the photo")
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                        .lineLimit(1)
                }
                Spacer(minLength: 8)
                Image(systemName: "chevron.up.chevron.down")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            .frame(maxWidth: .infinity, minHeight: 52, alignment: .leading)
            .padding(.horizontal, 14)
            .background(PlantyColor.surface, in: RoundedRectangle(cornerRadius: 15, style: .continuous))
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
    var guidance = "Fit the whole plant in the frame."
    var footnote = "One photo is enough."
    var consistentViewpoint = false
    let shutter: () -> Void

    @ScaledMetric(relativeTo: .largeTitle) private var shutterSize: CGFloat = 72

    var body: some View {
        VStack(spacing: 14) {
            preview

            Text(guidance)
                .font(.subheadline.weight(.medium))
                .foregroundStyle(PlantyColor.secondaryText)
                .frame(maxWidth: .infinity, alignment: .leading)

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
                            Circle().stroke(Color.white.opacity(0.9), lineWidth: 3)
                        }
                        .shadow(color: PlantyColor.pink.opacity(0.25), radius: 8, y: 3)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Take plant photo")

                Color.clear.frame(width: 52, height: 52)
                    .accessibilityHidden(true)
            }

            Text(footnote)
                .font(.caption)
                .foregroundStyle(PlantyColor.secondaryText)
        }
    }

    private var preview: some View {
        ZStack {
            if camera.availability == .ready {
                CameraPreview(session: camera.session)
            } else {
                LinearGradient(
                    colors: [PlantyColor.green.opacity(0.25), PlantyColor.surface],
                    startPoint: .top,
                    endPoint: .bottom
                )
                ProgressView()
            }
            if consistentViewpoint {
                VStack {
                    Spacer()
                    Capsule()
                        .strokeBorder(
                            Color.white.opacity(0.75),
                            style: StrokeStyle(lineWidth: 2, dash: [7, 6])
                        )
                        .frame(width: 190, height: 58)
                        .padding(.bottom, 42)
                }
                Image(systemName: "plus")
                    .font(.title2.weight(.light))
                    .foregroundStyle(Color.white.opacity(0.8))
            }
        }
        .frame(height: 390)
        .clipShape(RoundedRectangle(cornerRadius: 22, style: .continuous))
        .overlay {
            RoundedRectangle(cornerRadius: 22, style: .continuous)
                .stroke(PlantyColor.quietDecoration.opacity(0.16), lineWidth: 1)
        }
        .accessibilityHidden(true)
    }
}

struct CameraPermissionCard: View {
    @Binding var photoItem: PhotosPickerItem?

    var body: some View {
        StateMessage(
            title: "Camera access is off",
            message: "Allow camera access, or choose an existing photo. Both paths support the same Planty features.",
            accent: PlantyColor.pink,
            icon: "camera.fill"
        ) {
            Button("Allow camera access") {
                guard let url = URL(string: UIApplication.openSettingsURLString) else { return }
                UIApplication.shared.open(url)
            }
            .buttonStyle(PrimaryButtonStyle(color: PlantyColor.pink))

            PhotosPicker(selection: $photoItem, matching: .images) {
                Text("Choose from Photos")
                    .font(.headline)
                    .foregroundStyle(PlantyColor.foreground)
                    .frame(maxWidth: .infinity, minHeight: 50)
                    .background(PlantyColor.surface, in: RoundedRectangle(cornerRadius: 15))
            }
        }
    }
}

struct NoCameraCard: View {
    @Binding var photoItem: PhotosPickerItem?

    var body: some View {
        StateMessage(
            title: "No camera on this device",
            message: "Choose an existing photo instead. Everything else works the same.",
            accent: PlantyColor.cyan,
            icon: "camera.badge.ellipsis"
        ) {
            PhotosPicker(selection: $photoItem, matching: .images) {
                Text("Choose from Photos")
                    .font(.headline)
                    .foregroundStyle(PlantyColor.background)
                    .frame(maxWidth: .infinity, minHeight: 52)
                    .background(PlantyColor.cyan, in: RoundedRectangle(cornerRadius: 15))
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
            title: "That photo did not save",
            message: "It is still on this screen. Retry before closing so the observation is not lost.",
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
