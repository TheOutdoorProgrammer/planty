import SwiftUI

struct PlantStoryView: View {
    let plant: Plant
    let takePhoto: () -> Void

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 24) {
                plantHeader
                currentVerdict
                storyTimeline
            }
            .padding(.horizontal, 20)
            .padding(.top, 10)
            .padding(.bottom, 96)
        }
        .navigationTitle(plant.name)
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button(action: {}) {
                    Image(systemName: "ellipsis")
                        .frame(width: 44, height: 44)
                }
                .accessibilityLabel("More options for Mona")
            }
        }
        .safeAreaInset(edge: .bottom) {
            Button(action: takePhoto) {
                Label("Take today's photo", systemImage: "camera.fill")
            }
            .buttonStyle(PrimaryButtonStyle())
            .padding(.horizontal, 20)
            .padding(.vertical, 10)
            .background(.ultraThinMaterial)
        }
        .pageBackground()
    }

    private var plantHeader: some View {
        VStack(alignment: .leading, spacing: 14) {
            PlantPhoto(plant: plant, moment: .today, height: 260)

            HStack(alignment: .top) {
                VStack(alignment: .leading, spacing: 4) {
                    Text(plant.species)
                        .font(.title3.weight(.bold))
                    Label(plant.room, systemImage: "door.left.hand.open")
                        .font(.subheadline)
                        .foregroundStyle(PlantyColor.secondaryText)
                }

                Spacer(minLength: 12)
                OwnershipBadge(ownership: plant.ownership)
            }
        }
    }

    private var currentVerdict: some View {
        VStack(alignment: .leading, spacing: 12) {
            StatusPill(title: "Doing okay", icon: "checkmark.circle.fill", color: PlantyColor.green)
            Text("New growth is steady. The lower leaves are recovering after watering less.")
                .font(.title3.weight(.bold))
            Text("Last compared today at 8:04 AM")
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)

            DisclosureGroup {
                Text("The yellowing stopped after the pot spent less time wet. Recent cabinet temperature and humidity stayed within Mona's normal range.")
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
                    .padding(.top, 8)
            } label: {
                Text("Why Planty thinks this")
                    .font(.subheadline.weight(.semibold))
            }
            .tint(PlantyColor.cyan)
        }
        .plantyCard(border: PlantyColor.green.opacity(0.45))
    }

    private var storyTimeline: some View {
        VStack(alignment: .leading, spacing: 18) {
            VStack(alignment: .leading, spacing: 4) {
                Eyebrow(text: "Mona's story", color: PlantyColor.pink)
                Text("What changed, in pictures")
                    .font(.system(.title, design: .rounded, weight: .bold))
            }

            ForEach(Array(StoryEvent.monaStory.enumerated()), id: \.element.id) { index, event in
                StoryEventCard(
                    plant: plant,
                    event: event,
                    connectsToNext: index < StoryEvent.monaStory.count - 1
                )
            }
        }
    }
}

private struct StoryEventCard: View {
    let plant: Plant
    let event: StoryEvent
    let connectsToNext: Bool

    var body: some View {
        HStack(alignment: .top, spacing: 14) {
            VStack(spacing: 0) {
                Circle()
                    .fill(event.accent)
                    .frame(width: 14, height: 14)
                    .overlay {
                        Circle().stroke(PlantyColor.background, lineWidth: 3)
                    }

                if connectsToNext {
                    Rectangle()
                        .fill(PlantyColor.quietDecoration.opacity(0.55))
                        .frame(width: 2)
                        .frame(minHeight: 320)
                }
            }
            .accessibilityHidden(true)

            VStack(alignment: .leading, spacing: 12) {
                VStack(alignment: .leading, spacing: 4) {
                    Eyebrow(text: event.date, color: event.accent)
                    Text(event.title)
                        .font(.title3.weight(.bold))
                }

                PlantPhoto(plant: plant, moment: event.moment, height: 205)
                Text(event.narrative)
                    .font(.body)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            .padding(.bottom, connectsToNext ? 8 : 0)
        }
    }
}

#Preview("Plant story") {
    NavigationStack {
        PlantStoryView(plant: .mona, takePhoto: {})
    }
    .preferredColorScheme(.dark)
}
