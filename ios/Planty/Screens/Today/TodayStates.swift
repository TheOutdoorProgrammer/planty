import SwiftUI

/// Calm is compact and confidence-building: conclusion, freshness, and the one
/// optional action all fit in one glance instead of making the user scroll past
/// a mascot before learning whether anything needs doing.
struct CalmHero: View {
    let summary: CalmSummary
    let takePhoto: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            HStack(alignment: .top, spacing: 12) {
                VStack(alignment: .leading, spacing: 7) {
                    Eyebrow(text: "All clear", color: PlantyColor.green)
                    Text(summary.headline)
                        .font(.largeTitle.weight(.bold))
                    Text(summary.subhead)
                        .font(.body)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
                Spacer(minLength: 4)
                Image(systemName: "leaf.circle.fill")
                    .font(.system(size: 54))
                    .foregroundStyle(PlantyColor.green)
                    .accessibilityHidden(true)
            }

            Label(summary.freshnessLine, systemImage: "checkmark.circle.fill")
                .font(.footnote.weight(.semibold))
                .foregroundStyle(PlantyColor.green)
                .accessibilityLabel("\(summary.freshnessLine). This is how fresh the evidence is.")

            VStack(alignment: .leading, spacing: 5) {
                Text(summary.reassuranceTitle)
                    .font(.headline)
                Text(summary.reassuranceBody)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            .padding(.top, 2)

            Button("Take a photo", action: takePhoto)
                .buttonStyle(SecondaryButtonStyle())
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: PlantyColor.green.opacity(0.2))
    }
}

struct StaleBanner: View {
    let summary: StaleSummary
    let checkConnections: () -> Void
    let takePhoto: () -> Void

    var body: some View {
        StateMessage(
            title: summary.headline,
            message: summary.body,
            accent: PlantyColor.yellow,
            icon: "clock.badge.exclamationmark.fill"
        ) {
            if let pendingLabel = summary.pendingLabel {
                Text(pendingLabel)
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            if let failureLabel = summary.failureLabel {
                Label(failureLabel, systemImage: "exclamationmark.triangle.fill")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(PlantyColor.yellow)
            }
            Button("Check connections", action: checkConnections)
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
            Button("Take a photo", action: takePhoto)
                .buttonStyle(SecondaryButtonStyle())
        }
    }
}

struct TodayErrorView: View {
    let error: PlantyError
    let retry: () -> Void
    let takePhoto: () -> Void

    var body: some View {
        StateMessage(
            title: "Today's check did not finish.",
            message: """
                Your saved photos and notes are still here. Planty will try \
                again, or you can take a photo now if something looks wrong.
                """,
            accent: PlantyColor.orange,
            icon: "arrow.trianglehead.clockwise"
        ) {
            if let detail = error.errorDescription {
                Text(detail)
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            Button("Try again", action: retry)
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
            Button("Take a photo", action: takePhoto)
                .buttonStyle(SecondaryButtonStyle())
        }
    }
}

struct EmptySetupView: View {
    @Environment(AppSession.self) private var session

    var body: some View {
        StateMessage(
            title: "Add your first plant",
            message: "Start with a photo. You can name it now or let Planty help after the picture.",
            accent: PlantyColor.green,
            icon: "leaf.fill"
        ) {
            Button("Take the first photo") { session.selectedTab = .snap }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.green))
        }
    }
}

struct LoadingColdView: View {
    var body: some View {
        HStack(spacing: 14) {
            ProgressView().tint(PlantyColor.green)
            VStack(alignment: .leading, spacing: 3) {
                Text("Checking on everyone…")
                    .font(.headline)
                Text("Reading the latest photos and sensor updates.")
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard()
        .accessibilityElement(children: .combine)
    }
}

struct LoadingWarmView: View {
    let checkedAt: Date

    var body: some View {
        HStack(alignment: .top, spacing: 10) {
            ProgressView().tint(PlantyColor.cyan)
            VStack(alignment: .leading, spacing: 3) {
                Text("Refreshing today's evidence")
                    .font(.subheadline.weight(.semibold))
                Text("Showing the last check from \(RelativeAge.dayAndTime(checkedAt, now: Date())).")
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: PlantyColor.cyan.opacity(0.2), padding: 14)
    }
}

struct UnconfiguredCard: View {
    let openSettings: () -> Void

    var body: some View {
        StateMessage(
            title: "Connect Planty",
            message: "Add the service address and token before the app can tell you what needs attention.",
            accent: PlantyColor.cyan,
            icon: "antenna.radiowaves.left.and.right"
        ) {
            Button("Open settings", action: openSettings)
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.cyan))
        }
    }
}
