import SwiftUI

struct TodayView: View {
    enum PreviewState: String, CaseIterable {
        case calm = "Calm"
        case action = "Needs care"
    }

    let openCapture: () -> Void
    @State private var previewState = PreviewState.calm
    @State private var completedAction = false

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 22) {
                    if previewState == .calm || completedAction {
                        calmContent
                    } else {
                        actionContent
                    }
                }
                .padding(.horizontal, 20)
                .padding(.top, 12)
                .padding(.bottom, 32)
            }
            .navigationTitle("Planty")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Menu {
                        Picker("Prototype state", selection: $previewState) {
                            ForEach(PreviewState.allCases, id: \.self) { state in
                                Text(state.rawValue).tag(state)
                            }
                        }
                    } label: {
                        Image(systemName: "circle.lefthalf.filled")
                            .frame(width: 44, height: 44)
                    }
                    .accessibilityLabel("Change prototype state")
                }
            }
            .onChange(of: previewState) {
                completedAction = false
            }
            .pageBackground()
        }
    }

    private var calmContent: some View {
        VStack(spacing: 22) {
            PlantyLogo()
                .padding(.top, 6)

            VStack(spacing: 8) {
                Text(completedAction ? "That's it." : "You're done.")
                    .font(.system(.largeTitle, design: .rounded, weight: .bold))
                    .multilineTextAlignment(.center)

                Text("Nothing needs you right now.")
                    .font(.title3)
                    .foregroundStyle(PlantyColor.secondaryText)
                    .multilineTextAlignment(.center)
            }

            Label("8 plants checked · Updated at 8:04 AM", systemImage: "checkmark.seal.fill")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(PlantyColor.green)
                .multilineTextAlignment(.center)

            VStack(alignment: .leading, spacing: 10) {
                Label("All quiet in the greenhouse", systemImage: "sparkles")
                    .font(.headline)
                Text("The next automatic check is tomorrow morning.")
                    .font(.body)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .plantyCard(border: PlantyColor.green.opacity(0.45))

            Button(action: openCapture) {
                Label("Take a photo anyway", systemImage: "camera.fill")
            }
            .buttonStyle(SecondaryButtonStyle())
        }
        .frame(maxWidth: .infinity)
    }

    private var actionContent: some View {
        VStack(alignment: .leading, spacing: 20) {
            VStack(alignment: .leading, spacing: 7) {
                Eyebrow(text: "Today", color: PlantyColor.orange)
                Text("One thing. You've got this.")
                    .font(.system(.largeTitle, design: .rounded, weight: .bold))
                Text("Everything else is okay.")
                    .font(.title3)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            CareActionCard(
                plant: .mona,
                handleNow: {
                    openCapture()
                },
                completeDemo: {
                    withAnimation(.snappy) {
                        completedAction = true
                    }
                }
            )

            Text("This is a normal watering, not an emergency.")
                .font(.footnote)
                .foregroundStyle(PlantyColor.secondaryText)
                .frame(maxWidth: .infinity, alignment: .center)
        }
    }
}

private struct CareActionCard: View {
    let plant: Plant
    let handleNow: () -> Void
    let completeDemo: () -> Void
    @State private var showsPostponeOptions = false

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            HStack(alignment: .top) {
                OwnershipBadge(ownership: plant.ownership)
                Spacer(minLength: 8)
                StatusPill(title: "Needs water", icon: "drop.fill", color: PlantyColor.orange)
            }

            HStack(spacing: 14) {
                PlantPhoto(plant: plant, moment: .today, height: 92)
                    .frame(width: 92)

                VStack(alignment: .leading, spacing: 4) {
                    Text(plant.name)
                        .font(.title2.weight(.bold))
                    Text(plant.species)
                        .font(.subheadline)
                        .foregroundStyle(PlantyColor.secondaryText)
                    Label(plant.room, systemImage: "door.left.hand.open")
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            }

            VStack(alignment: .leading, spacing: 8) {
                Text("Give it a slow drink. Stop when water reaches the tray.")
                    .font(.title3.weight(.bold))
                Text("The soil has stayed dry for two days, and the leaves look unchanged.")
                    .font(.body)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            Button(action: handleNow) {
                Label("I'm here", systemImage: "location.fill")
            }
            .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))

            Button {
                showsPostponeOptions = true
            } label: {
                Text("Not now")
                    .font(.subheadline.weight(.semibold))
                    .frame(maxWidth: .infinity, minHeight: 44)
            }
            .foregroundStyle(PlantyColor.secondaryText)
            .accessibilityHint("Choose when Planty should remind you")
        }
        .plantyCard(border: PlantyColor.purple.opacity(0.7))
        .confirmationDialog("When should Planty remind you?", isPresented: $showsPostponeOptions) {
            Button("In 1 hour", action: {})
            Button("Later today", action: {})
            Button("I already handled it", action: completeDemo)
            Button("Cancel", role: .cancel, action: {})
        } message: {
            Text("Postponing does not mark Mona's watering complete.")
        }
    }
}

#Preview("Today — calm") {
    TodayView(openCapture: {})
        .preferredColorScheme(.dark)
}
