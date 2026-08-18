import SwiftUI

struct CaptureFlowView: View {
    private enum CaptureStage {
        case ready
        case captured
    }

    @State private var stage = CaptureStage.ready
    @State private var note = ""
    @State private var savedMessage: String?

    private let plant = Plant.mona

    var body: some View {
        ScrollView {
            VStack(spacing: 18) {
                plantSelector
                cameraSurface

                if stage == .captured {
                    capturedActions
                        .transition(.move(edge: .bottom).combined(with: .opacity))
                }
            }
            .padding(.horizontal, 20)
            .padding(.top, 8)
            .padding(.bottom, 32)
        }
        .navigationTitle("Snap")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    withAnimation(.snappy) {
                        stage = .ready
                    }
                } label: {
                    Text(stage == .captured ? "Retake" : "Flash")
                }
                .frame(minHeight: 44)
                .accessibilityLabel(stage == .captured ? "Retake photo" : "Flash settings")
            }
        }
        .overlay(alignment: .top) {
            if let savedMessage {
                SaveToast(message: savedMessage)
                    .padding(.top, 8)
                    .transition(.move(edge: .top).combined(with: .opacity))
            }
        }
        .safeAreaInset(edge: .bottom) {
            if stage == .ready {
                shutterControls
                    .padding(.horizontal, 20)
                    .padding(.vertical, 10)
                    .background(.ultraThinMaterial)
                    .transition(.opacity)
            }
        }
        .pageBackground()
    }

    private var plantSelector: some View {
        Button(action: {}) {
            HStack(spacing: 10) {
                Image(systemName: "leaf.fill")
                    .foregroundStyle(PlantyColor.green)
                VStack(alignment: .leading, spacing: 2) {
                    Text(plant.name)
                        .font(.headline)
                    Text("\(plant.ownership.label) · \(plant.room)")
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
                Spacer()
                Image(systemName: "chevron.up.chevron.down")
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            .padding(.horizontal, 16)
            .frame(minHeight: 56)
            .background(PlantyColor.surface, in: Capsule())
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Selected plant, Mona, Maya's plant in the living room. Change plant.")
    }

    private var cameraSurface: some View {
        ZStack {
            PlantPhoto(plant: plant, moment: stage == .ready ? .camera : .today, height: 430)

            if stage == .ready {
                RoundedRectangle(cornerRadius: 70, style: .continuous)
                    .stroke(PlantyColor.foreground.opacity(0.55), style: StrokeStyle(lineWidth: 2, dash: [8, 8]))
                    .padding(.horizontal, 58)
                    .padding(.vertical, 52)
                    .accessibilityHidden(true)

                VStack {
                    Spacer()
                    Text("Fit the whole plant in the frame.")
                        .font(.subheadline.weight(.semibold))
                        .multilineTextAlignment(.center)
                        .padding(.horizontal, 14)
                        .padding(.vertical, 9)
                        .background(PlantyColor.background.opacity(0.84), in: Capsule())
                        .padding(.bottom, 22)
                }
            }
        }
        .accessibilityElement(children: .combine)
    }

    private var shutterControls: some View {
        VStack(spacing: 14) {
            HStack {
                Button(action: {}) {
                    Image(systemName: "photo.on.rectangle")
                        .font(.title2)
                        .frame(width: 52, height: 52)
                        .background(PlantyColor.surface, in: Circle())
                }
                .accessibilityLabel("Choose from Photos")

                Spacer()

                Button {
                    withAnimation(.snappy) {
                        stage = .captured
                    }
                } label: {
                    Circle()
                        .fill(PlantyColor.foreground)
                        .frame(width: 76, height: 76)
                        .overlay {
                            Circle()
                                .stroke(PlantyColor.pink, lineWidth: 6)
                                .padding(5)
                        }
                }
                .accessibilityLabel("Take plant photo")
                .accessibilityHint("Captures Mona and opens quick record options")

                Spacer()

                Button(action: {}) {
                    Image(systemName: "camera.rotate.fill")
                        .font(.title2)
                        .frame(width: 52, height: 52)
                        .background(PlantyColor.surface, in: Circle())
                }
                .accessibilityLabel("Switch camera")
            }
            .padding(.horizontal, 14)

            Text("One photo is enough.")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(PlantyColor.secondaryText)
        }
    }

    private var capturedActions: some View {
        VStack(alignment: .leading, spacing: 16) {
            VStack(alignment: .leading, spacing: 5) {
                Eyebrow(text: "Quick record", color: PlantyColor.pink)
                Text("What happened here?")
                    .font(.system(.title, design: .rounded, weight: .bold))
            }

            Button {
                showSaved("Watering added to Mona's story")
            } label: {
                QuickActionRow(
                    title: "Watered by hand",
                    subtitle: "Save photo and care action",
                    icon: "drop.fill",
                    color: PlantyColor.cyan
                )
            }
            .buttonStyle(.plain)

            Divider().overlay(PlantyColor.quietDecoration)

            Button {
                showSaved("Repotting added to Mona's story")
            } label: {
                QuickActionRow(
                    title: "Repotted",
                    subtitle: "Save photo and mark a major change",
                    icon: "shippingbox.fill",
                    color: PlantyColor.orange
                )
            }
            .buttonStyle(.plain)

            Divider().overlay(PlantyColor.quietDecoration)

            NavigationLink {
                DiagnosisView(plant: plant)
            } label: {
                QuickActionRow(
                    title: "Something looks off",
                    subtitle: "Compare this photo with Mona's story",
                    icon: "bubble.left.and.bubble.right.fill",
                    color: PlantyColor.pink
                )
            }
            .buttonStyle(.plain)

            TextField("Add a short note (optional)", text: $note, axis: .vertical)
                .textFieldStyle(.plain)
                .padding(14)
                .background(PlantyColor.background.opacity(0.62), in: RoundedRectangle(cornerRadius: 14))
                .accessibilityHint("This field is optional")

            Button {
                showSaved("Photo added to Mona's story")
            } label: {
                Text("Save photo only")
            }
            .buttonStyle(SecondaryButtonStyle())
        }
        .plantyCard(border: PlantyColor.pink.opacity(0.45))
    }

    private func showSaved(_ message: String) {
        withAnimation(.snappy) {
            savedMessage = message
            stage = .ready
            note = ""
        }
    }
}

private struct SaveToast: View {
    let message: String

    var body: some View {
        Label(message, systemImage: "checkmark.circle.fill")
            .font(.subheadline.weight(.semibold))
            .foregroundStyle(PlantyColor.background)
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
            .background(PlantyColor.green, in: Capsule())
            .shadow(color: PlantyColor.background.opacity(0.35), radius: 10, y: 4)
            .accessibilityAddTraits(.isStaticText)
    }
}

#Preview("Capture") {
    NavigationStack {
        CaptureFlowView()
    }
    .preferredColorScheme(.dark)
}
