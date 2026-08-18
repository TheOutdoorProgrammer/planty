import SwiftUI

struct DiagnosisView: View {
    let plant: Plant
    @State private var message = ""
    @State private var actionComplete = false

    var body: some View {
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 18) {
                userObservation
                assistantFinding

                if actionComplete {
                    Label("Tray emptied · added to Mona's story", systemImage: "checkmark.circle.fill")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(PlantyColor.green)
                        .frame(maxWidth: .infinity, alignment: .center)
                        .padding(.vertical, 8)
                        .transition(.scale.combined(with: .opacity))
                }

                suggestedQuestions
            }
            .padding(.horizontal, 20)
            .padding(.top, 12)
            .padding(.bottom, 110)
        }
        .navigationTitle(plant.name)
        .navigationBarTitleDisplayMode(.inline)
        .safeAreaInset(edge: .bottom) {
            composer
        }
        .pageBackground()
    }

    private var userObservation: some View {
        VStack(alignment: .trailing, spacing: 10) {
            PlantPhoto(plant: plant, moment: .today, height: 190)
                .frame(maxWidth: 280)
            Text("Something looks off.")
                .font(.body.weight(.semibold))
                .padding(.horizontal, 15)
                .padding(.vertical, 11)
                .background(PlantyColor.pink, in: RoundedRectangle(cornerRadius: 18, style: .continuous))
                .foregroundStyle(PlantyColor.background)
        }
        .frame(maxWidth: .infinity, alignment: .trailing)
    }

    private var assistantFinding: some View {
        VStack(alignment: .leading, spacing: 18) {
            HStack(spacing: 10) {
                PlantyLogo(size: 46)
                VStack(alignment: .leading, spacing: 2) {
                    Text("PLANTY LOOKED CLOSER")
                        .font(.caption.weight(.bold))
                        .tracking(0.8)
                        .foregroundStyle(PlantyColor.pink)
                    Text("Compared with 6 earlier photos")
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            }

            VStack(alignment: .leading, spacing: 9) {
                Text("The lower yellowing has spread since July 20.")
                    .font(.title3.weight(.bold))
                Text("Most likely, Mona has been staying wet too long. The pattern fits overwatering better than low light.")
                    .font(.body)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            VStack(alignment: .leading, spacing: 8) {
                Eyebrow(text: "Do this today", color: PlantyColor.orange)
                Text("Do not water it. Empty any water sitting in the tray.")
                    .font(.title3.weight(.bold))
            }
            .padding(16)
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(PlantyColor.orange.opacity(0.13), in: RoundedRectangle(cornerRadius: 16))

            Label("I will compare another photo in 3 days.", systemImage: "calendar.badge.clock")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(PlantyColor.cyan)

            DisclosureGroup {
                Text("7 photos and 14 days of moisture readings support this finding. Image change is the stronger signal; moisture duration supports it.")
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
                    .padding(.top, 8)
            } label: {
                Text("Why Planty thinks this")
                    .font(.subheadline.weight(.semibold))
            }
            .tint(PlantyColor.cyan)

            if !actionComplete {
                Button {
                    withAnimation(.snappy) {
                        actionComplete = true
                    }
                } label: {
                    Label("I emptied the tray", systemImage: "checkmark")
                }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
            }
        }
        .plantyCard(border: PlantyColor.orange.opacity(0.5))
    }

    private var suggestedQuestions: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text("Ask about this finding")
                .font(.headline)

            ScrollView(.horizontal, showsIndicators: false) {
                HStack(spacing: 10) {
                    suggestion("Show me what changed")
                    suggestion("Could this be pests?")
                    suggestion("What would make this urgent?")
                }
            }
        }
    }

    private func suggestion(_ text: String) -> some View {
        Button(text, action: {})
            .font(.subheadline.weight(.semibold))
            .foregroundStyle(PlantyColor.foreground)
            .padding(.horizontal, 14)
            .frame(minHeight: 44)
            .background(PlantyColor.surface, in: Capsule())
    }

    private var composer: some View {
        HStack(spacing: 10) {
            TextField("Ask a follow-up…", text: $message, axis: .vertical)
                .textFieldStyle(.plain)
                .padding(.horizontal, 16)
                .frame(minHeight: 48)
                .background(PlantyColor.surface, in: Capsule())

            Button(action: {}) {
                Image(systemName: message.isEmpty ? "camera.fill" : "arrow.up")
                    .font(.headline)
                    .foregroundStyle(PlantyColor.background)
                    .frame(width: 48, height: 48)
                    .background(PlantyColor.pink, in: Circle())
            }
            .accessibilityLabel(message.isEmpty ? "Add another photo" : "Send follow-up")
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 10)
        .background(.ultraThinMaterial)
    }
}

#Preview("Diagnosis") {
    NavigationStack {
        DiagnosisView(plant: .mona)
    }
    .preferredColorScheme(.dark)
}
