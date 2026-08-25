import SwiftUI

struct ScheduledJobsScreen: View {
    @Environment(AppSession.self) private var session

    private var store: ScheduledJobStore { session.scheduledJobs }
    private var categories: [String] {
        var seen: Set<String> = []
        return store.jobs.compactMap { seen.insert($0.category).inserted ? $0.category : nil }
    }

    var body: some View {
        List {
            if let error = store.error {
                Section {
                    Label(
                        error.errorDescription ?? "Scheduled jobs could not be reached.",
                        systemImage: "exclamationmark.triangle.fill"
                    )
                    .foregroundStyle(PlantyColor.orange)

                    Button("Try again") { Task { await store.load() } }
                }
            }

            if !store.hasLoaded {
                HStack(spacing: 12) {
                    ProgressView()
                    Text("Reading Planty's schedules…")
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            } else if store.jobs.isEmpty && store.error == nil {
                ContentUnavailableView(
                    "No scheduled jobs",
                    systemImage: "calendar.badge.exclamationmark",
                    description: Text("This Planty service has not exposed manual runs.")
                )
            }

            ForEach(categories, id: \.self) { category in
                Section(category) {
                    ForEach(store.jobs.filter { $0.category == category }) { job in
                        ScheduledJobRow(job: job)
                    }
                }
            }
        }
        .scrollContentBackground(.hidden)
        .plantyPage()
        .navigationTitle("Scheduled jobs")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await store.load() }
        .task { await store.loadIfNeeded() }
    }
}

private struct ScheduledJobRow: View {
    @Environment(AppSession.self) private var session
    let job: ScheduledJob

    private var isRunning: Bool { session.scheduledJobs.isRunning(job.id) }

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .firstTextBaseline, spacing: 12) {
                    title
                    Spacer(minLength: 4)
                    runButton
                }
                VStack(alignment: .leading, spacing: 10) {
                    title
                    runButton
                }
            }

            Text(job.purpose)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)

            HStack(spacing: 10) {
                Label(job.cadence, systemImage: "calendar")
                if job.suspended {
                    Label("Suspended", systemImage: "pause.circle.fill")
                        .foregroundStyle(PlantyColor.yellow)
                }
            }
            .font(.caption)
            .foregroundStyle(PlantyColor.secondaryText)

            if let run = job.latestRun {
                ScheduledRunStatus(run: run)
            }
        }
        .padding(.vertical, 4)
    }

    private var title: some View {
        Text(job.name)
            .font(.headline)
    }

    private var runButton: some View {
        Button(isRunning ? "Running…" : "Run now") {
            Task { await session.scheduledJobs.run(job.id) }
        }
        .buttonStyle(.bordered)
        .tint(PlantyColor.green)
        .disabled(isRunning || job.suspended)
        .accessibilityLabel(isRunning ? "\(job.name) is running" : "Run \(job.name) now")
    }
}

private struct ScheduledRunStatus: View {
    let run: ScheduledJobRun

    var body: some View {
        HStack(alignment: .firstTextBaseline, spacing: 7) {
            Image(systemName: icon)
            Text(label)
            if let date = run.completedAt ?? run.startedAt {
                Text("· \(RelativeAge.dayAndTime(date, now: Date()))")
            }
        }
        .font(.caption.weight(.semibold))
        .foregroundStyle(color)
        .accessibilityElement(children: .combine)
    }

    private var label: String {
        switch run.state {
        case .queued: "Queued"
        case .running: "Running"
        case .succeeded: "Last run succeeded"
        case .failed: "Last run failed"
        case .unknown: "Status unavailable"
        }
    }

    private var icon: String {
        switch run.state {
        case .queued: "clock.fill"
        case .running: "arrow.trianglehead.2.clockwise.rotate.90"
        case .succeeded: "checkmark.circle.fill"
        case .failed: "xmark.octagon.fill"
        case .unknown: "questionmark.circle.fill"
        }
    }

    private var color: Color {
        switch run.state {
        case .queued, .running: PlantyColor.cyan
        case .succeeded: PlantyColor.green
        case .failed: PlantyColor.red
        case .unknown: PlantyColor.secondaryText
        }
    }
}
