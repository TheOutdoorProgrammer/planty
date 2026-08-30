import SwiftUI

/// Calibration is what turns a probe's private numbers into a decision Planty
/// is allowed to act on, so this sheet spends most of its room saying when to
/// take each reading rather than asking for two numbers.
struct CalibrateSensorSheet: View {
    let link: SensorLink
    let latest: Reading?
    let save: (SensorCalibration) async -> PlantyError?

    @Environment(\.dismiss) private var dismiss
    @State private var draft: SensorCalibrationDraft
    @State private var action = AsyncSheetAction()

    init(
        link: SensorLink,
        latest: Reading?,
        save: @escaping (SensorCalibration) async -> PlantyError?
    ) {
        self.link = link
        self.latest = latest
        self.save = save
        _draft = State(initialValue: SensorCalibrationDraft(link: link))
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text(link.haEntityID)
                        .font(.headline)
                    Text(link.role.label)
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                } footer: {
                    Text("""
                        These two numbers only mean anything for this probe. \
                        Two probes in two pots never read the same.
                        """)
                }

                Section {
                    if let latest {
                        HStack(spacing: 14) {
                            Image(systemName: "drop.fill")
                                .font(.title2.weight(.semibold))
                                .foregroundStyle(PlantyColor.cyan)
                                .frame(width: 44, height: 44)
                                .background(PlantyColor.cyan.opacity(0.14), in: Circle())
                            VStack(alignment: .leading, spacing: 3) {
                                Text(latest.displayValue)
                                    .font(.title2.weight(.bold).monospacedDigit())
                                Text("Reported \(RelativeAge.dayAndTime(latest.takenAt, now: Date()))")
                                    .font(.caption)
                                    .foregroundStyle(PlantyColor.secondaryText)
                            }
                        }
                        .accessibilityElement(children: .combine)
                    } else {
                        Label("Waiting for the first reading", systemImage: "clock.badge.questionmark")
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                } header: {
                    Text("Current reading")
                } footer: {
                    if latest == nil {
                        Text("Planty will show the probe value here after the next sensor ingest.")
                    }
                }

                Section {
                    TextField("What it reads dry", text: $draft.dry)
                        .keyboardType(.decimalPad)
                } header: {
                    calibrationHeader("Dry", isSaved: link.dryBaseline != nil)
                } footer: {
                    Text("Take this just before you water, when the soil is as dry as you would ever let it get.")
                }

                Section {
                    TextField("What it reads wet", text: $draft.wet)
                        .keyboardType(.decimalPad)
                } header: {
                    calibrationHeader("Wet", isSaved: link.wetBaseline != nil)
                } footer: {
                    Text("Take this straight after a thorough watering, once it has soaked all the way through.")
                }

                if let backwards = proposed, !backwards.readsTheRightWayRound {
                    Section {
                        Label(
                            """
                            Wet has to read higher than dry. The other way round, \
                            a soaked pot reads as bone dry and gets watered again.
                            """,
                            systemImage: "exclamationmark.triangle.fill"
                        )
                        .foregroundStyle(PlantyColor.orange)
                    }
                }

                if link.isCalibrated {
                    Section {
                        Text("Saved values are loaded above. Saving replaces both baselines.")
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                }

                if let failure = action.error {
                    Section {
                        Label {
                            Text("""
                                \(failure.errorDescription ?? "The service did not answer.") \
                                Nothing was saved; your readings are still here.
                                """)
                        } icon: {
                            Image(systemName: "exclamationmark.triangle.fill")
                        }
                        .foregroundStyle(PlantyColor.orange)
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Calibrate")
            .navigationBarTitleDisplayMode(.inline)
            .interactiveDismissDisabled(action.isRunning)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .disabled(action.isRunning)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(action.isRunning ? "Saving…" : "Save") {
                        Task { await attemptSave() }
                    }
                    .disabled(proposed?.readsTheRightWayRound != true || action.isRunning)
                }
            }
        }
    }

    /// Dismisses only once the service accepted the baselines. A failure keeps
    /// the sheet open with both readings intact.
    private func attemptSave() async {
        guard let ready = proposed, ready.readsTheRightWayRound else { return }
        guard await action.perform({ await save(ready) }) else { return }
        dismiss()
    }

    private var proposed: SensorCalibration? {
        draft.proposed
    }

    private func calibrationHeader(_ title: String, isSaved: Bool) -> some View {
        HStack {
            Text(title)
            Spacer()
            if isSaved {
                Label("Saved", systemImage: "checkmark.circle.fill")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(PlantyColor.green)
                    .textCase(nil)
            }
        }
    }
}
