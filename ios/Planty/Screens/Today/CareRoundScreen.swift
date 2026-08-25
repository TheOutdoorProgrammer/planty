import SwiftUI

struct CareRoundScreen: View {
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss

    private var store: TodayStore { session.today }
    private var groups: [CareRoundGroup] {
        CareRoundPlanner.groups(
            digest: store.digest,
            resolvedVerdicts: store.resolvedIDs,
            resolvedReminders: store.resolvedReminderOccurrenceIDs
        )
    }
    private var remaining: Int { groups.reduce(0) { $0 + $1.count } }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 20) {
                    if store.isLoading && store.digest == nil {
                        LoadingColdView()
                    } else if groups.isEmpty {
                        StateMessage(
                            title: "Round complete",
                            message: "There is no due care left to carry between rooms.",
                            accent: PlantyColor.green,
                            icon: "checkmark.circle.fill"
                        ) { EmptyView() }
                    } else {
                        VStack(alignment: .leading, spacing: 6) {
                            Text("\(remaining) task\(remaining == 1 ? "" : "s") in \(groups.count) location\(groups.count == 1 ? "" : "s")")
                                .font(.title2.weight(.bold))
                            Text("Urgent locations come first. Within each location, Planty keeps the same evidence and actions shown on Today.")
                                .font(.subheadline)
                                .foregroundStyle(PlantyColor.secondaryText)
                        }
                        .plantyCard()

                        ForEach(groups) { group in
                            VStack(alignment: .leading, spacing: 12) {
                                SectionHeading(group.room, detail: "\(group.count) remaining")
                                ForEach(group.reminders) { reminder in
                                    TodayReminderCard(
                                        occurrence: reminder,
                                        isResolving: store.resolvingReminderOccurrenceIDs.contains(reminder.occurrenceID)
                                    ) { disposition in
                                        Task { await store.resolve(reminder, as: disposition) }
                                    }
                                }
                                ForEach(group.entries) { entry in
                                    NavigationLink {
                                        TodayActionScreen(entry: entry)
                                    } label: {
                                        CareCard(entry: entry)
                                    }
                                    .buttonStyle(.plain)
                                }
                            }
                        }
                    }
                }
                .padding(.horizontal, 16)
                .padding(.vertical, 18)
                .frame(maxWidth: .infinity, alignment: .leading)
                .plantyReadableContent()
            }
            .plantyPage()
            .navigationTitle("Care round")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
            .task { await store.load() }
            .refreshable { await store.load() }
        }
    }
}
