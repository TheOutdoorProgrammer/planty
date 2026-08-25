import SwiftUI

/// A scheduled care occurrence is actionable on the Today screen itself. The
/// confirmation copy matches the observation kind that will be written, so a
/// misting reminder can never quietly become a generic acknowledgement.
struct TodayReminderCard: View {
    let occurrence: DueReminder
    let isResolving: Bool
    let resolve: (ReminderDisposition) -> Void

    @State private var confirmingMiss = false

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            PlantPhotoView(plant: occurrence.plant, height: 132, opensFullScreen: false)
                .allowsHitTesting(false)

            HStack(spacing: 10) {
                OwnershipBadge(plant: occurrence.plant)
                Label("Scheduled", systemImage: "bell.fill")
                    .font(.caption.weight(.bold))
                    .foregroundStyle(PlantyColor.cyan)
                Spacer(minLength: 0)
            }

            VStack(alignment: .leading, spacing: 5) {
                Text(occurrence.reminder.kind.instruction)
                    .font(.title3.weight(.bold))
                    .foregroundStyle(PlantyColor.foreground)
                Text(occurrence.plant.commonName)
                    .font(.headline)
                Text(occurrence.dueLine)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            if let note = occurrence.reminder.note, !note.isEmpty {
                Text(note)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            ViewThatFits {
                HStack(spacing: 10) {
                    completionButton
                    missedButton
                }
                VStack(spacing: 10) {
                    completionButton
                    missedButton
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: PlantyColor.cyan.opacity(0.3))
        .accessibilityElement(children: .contain)
        .confirmationDialog(
            "Mark this reminder missed?",
            isPresented: $confirmingMiss,
            titleVisibility: .visible
        ) {
            Button("Mark missed") { resolve(.missed) }
            Button("Keep it due", role: .cancel) {}
        } message: {
            Text("Planty will close only this scheduled occurrence and record no care.")
        }
    }

    private var completionButton: some View {
        Button { resolve(.completed) } label: {
            if isResolving {
                HStack(spacing: 8) {
                    ProgressView()
                    Text("Saving…")
                }
                .frame(maxWidth: .infinity)
            } else {
                Label(
                    occurrence.completionLabel,
                    systemImage: occurrence.reminder.kind.symbol
                )
                .frame(maxWidth: .infinity)
            }
        }
        .buttonStyle(PrimaryButtonStyle(color: PlantyColor.green))
        .disabled(isResolving)
    }

    private var missedButton: some View {
        Button {
            confirmingMiss = true
        } label: {
            Label("I missed this one", systemImage: "calendar.badge.exclamationmark")
                .frame(maxWidth: .infinity)
        }
        .buttonStyle(SecondaryButtonStyle())
        .disabled(isResolving)
    }
}
