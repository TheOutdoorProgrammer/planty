import SwiftUI

/// The state the whole screen is designed around. Says how many plants were
/// checked and when, because a calm claim with no freshness earns no trust.
struct CalmHero: View {
    let summary: CalmSummary
    let takePhoto: () -> Void

    var body: some View {
        VStack(spacing: 18) {
            PlantyMascot()

            VStack(spacing: 8) {
                Text(summary.headline)
                    .font(.largeTitle.weight(.bold))
                Text(summary.subhead)
                    .font(.title3)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            .multilineTextAlignment(.center)

            Text(summary.freshnessLine)
                .font(.footnote.weight(.semibold))
                .foregroundStyle(PlantyColor.green)
                .accessibilityLabel("\(summary.freshnessLine). This is how fresh the evidence is.")

            VStack(alignment: .leading, spacing: 6) {
                Text(summary.reassuranceTitle)
                    .font(.headline)
                Text(summary.reassuranceBody)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .plantyCard()

            Button("Take a photo anyway", action: takePhoto)
                .buttonStyle(SecondaryButtonStyle())
        }
        .frame(maxWidth: .infinity)
    }
}

/// Never dressed as calm, never dressed as danger. It says only that Planty
/// cannot honestly speak for the plants right now.
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
            Button("Check connections", action: checkConnections)
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.yellow))
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
        VStack(spacing: 18) {
            PlantyMascot(wobbles: false)
            StateMessage(
                title: "No plants to worry about yet.",
                message: """
                    Add the first one with a photo. If you do not know its name, \
                    Planty can help after the picture.
                    """,
                accent: PlantyColor.pink,
                icon: "leaf.fill"
            ) {
                Button("Add a plant") { session.selectedTab = .snap }
                    .buttonStyle(PrimaryButtonStyle())
            }
        }
    }
}

struct LoadingColdView: View {
    var body: some View {
        VStack(spacing: 16) {
            PlantyMascot(wobbles: false)
            Text("Checking on everyone…")
                .font(.title2.weight(.bold))
            Text("Planty is reading the latest photos and sensor updates.")
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity)
        .accessibilityElement(children: .combine)
    }
}

/// Keeps the previous verdicts on screen and names how old they are, rather
/// than replacing real information with a spinner.
struct LoadingWarmView: View {
    let checkedAt: Date

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Label("Checking today's evidence…", systemImage: "arrow.trianglehead.clockwise")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(PlantyColor.cyan)
            Text("Showing the last check from \(RelativeAge.dayAndTime(checkedAt, now: Date())).")
                .font(.footnote)
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: PlantyColor.cyan.opacity(0.35), padding: 14)
    }
}

struct UnconfiguredCard: View {
    let openSettings: () -> Void

    var body: some View {
        StateMessage(
            title: "Planty is not connected yet.",
            message: """
                Point the app at your Planty service and paste its token. \
                Until then there is no evidence to be calm about.
                """,
            accent: PlantyColor.cyan,
            icon: "antenna.radiowaves.left.and.right"
        ) {
            Button("Open settings", action: openSettings)
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.cyan))
        }
    }
}
