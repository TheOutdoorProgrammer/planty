import SwiftUI

/// Calibration is what turns a probe's private numbers into a decision Planty
/// is allowed to act on, so this sheet spends most of its room saying when to
/// take each reading rather than asking for two numbers.
struct CalibrateSensorSheet: View {
    let link: SensorLink
    let save: (SensorCalibration) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var dry = ""
    @State private var wet = ""

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
                    TextField("What it reads dry", text: $dry)
                        .keyboardType(.decimalPad)
                } header: {
                    Text("Dry")
                } footer: {
                    Text("Take this just before you water, when the soil is as dry as you would ever let it get.")
                }

                Section {
                    TextField("What it reads wet", text: $wet)
                        .keyboardType(.decimalPad)
                } header: {
                    Text("Wet")
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
                        Text("Already calibrated. Saving replaces both baselines.")
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Calibrate")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        if let ready = proposed, ready.readsTheRightWayRound {
                            save(ready)
                        }
                    }
                    .disabled(proposed?.readsTheRightWayRound != true)
                }
            }
        }
    }

    private var proposed: SensorCalibration? {
        SensorCalibration(dry: dry, wet: wet)
    }
}
