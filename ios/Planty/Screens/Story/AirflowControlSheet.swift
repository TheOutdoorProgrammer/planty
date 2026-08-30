import SwiftUI

struct AirflowControlSheet: View {
    let plant: Plant
    let onChanged: () async -> Void

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var selectedActuatorID: UUID?
    @State private var duration = ActuatorRunDuration.tenMinutes
    @State private var failure: PlantyError?

    private var actuators: [Actuator] {
        session.actuators.registered.assigned(to: plant.id)
    }

    private var selectedActuator: Actuator? {
        if let selectedActuatorID,
           let selected = actuators.first(where: { $0.id == selectedActuatorID }) {
            return selected
        }
        return actuators.first
    }

    private var activeLease: ActuatorLease? {
        guard let selectedActuator else { return nil }
        return session.actuators.leases[selectedActuator.id]
    }

    var body: some View {
        NavigationStack {
            Form {
                if let failure {
                    Section {
                        SheetErrorRow(headline: "The fan was not changed.", error: failure)
                    }
                }

                if actuators.count > 1 {
                    Section("Fan") {
                        Picker("Fan", selection: $selectedActuatorID) {
                            ForEach(actuators) { actuator in
                                Text(actuator.name).tag(Optional(actuator.id))
                            }
                        }
                    }
                }

                if let actuator = selectedActuator {
                    airflowSection(actuator)
                } else {
                    ContentUnavailableView(
                        "No fan assigned",
                        systemImage: "fan.slash",
                        description: Text("Assign a fan to this plant in Settings first.")
                    )
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Airflow")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Done") { dismiss() }
                }
            }
        }
        .task {
            if selectedActuatorID == nil {
                selectedActuatorID = actuators.first {
                    session.actuators.leases[$0.id]?.isActive == true
                }?.id ?? actuators.first?.id
            }
        }
    }

    @ViewBuilder
    private func airflowSection(_ actuator: Actuator) -> some View {
        if let lease = activeLease, lease.isActive {
            runningSection(actuator, lease: lease)
        } else {
            stoppedSection(actuator)
        }
    }

    private func runningSection(_ actuator: Actuator, lease: ActuatorLease) -> some View {
        Section {
            TimelineView(.periodic(from: .now, by: 1)) { context in
                HStack(spacing: 14) {
                    Image(systemName: "fan.fill")
                        .font(.title2.weight(.semibold))
                        .foregroundStyle(PlantyColor.green)
                        .symbolEffect(.rotate, options: .repeating, isActive: lease.deadline > context.date)
                    VStack(alignment: .leading, spacing: 3) {
                        Text("\(actuator.name) is running").font(.headline)
                        Text(timeRemaining(until: lease.deadline, now: context.date))
                            .font(.subheadline.monospacedDigit())
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                }
                .accessibilityElement(children: .combine)
            }
            Button("Stop airflow", role: .destructive) { Task { await stop(actuator) } }
                .disabled(session.actuators.controlling.contains(actuator.id))
        } footer: {
            Text(sharedImpact(for: actuator, alreadyRecorded: true))
        }
    }

    private func stoppedSection(_ actuator: Actuator) -> some View {
        Section {
            Label(actuator.name, systemImage: "fan.fill").font(.headline)
            Picker("Run for", selection: $duration) {
                ForEach(ActuatorRunDuration.allCases) { duration in
                    Text(duration.label).tag(duration)
                }
            }
            Button { Task { await start(actuator) } } label: {
                HStack {
                    if session.actuators.controlling.contains(actuator.id) {
                        ProgressView()
                    } else {
                        Image(systemName: "fan.fill")
                    }
                    Text("Start airflow")
                }
                .frame(maxWidth: .infinity)
            }
            .buttonStyle(PrimaryButtonStyle(color: PlantyColor.green))
            .disabled(session.actuators.controlling.contains(actuator.id))
        } header: {
            Text("Run fan")
        } footer: {
            Text(sharedImpact(for: actuator, alreadyRecorded: false))
        }
    }

    private func start(_ actuator: Actuator) async {
        failure = await session.actuators.start(actuator, durationSeconds: duration.rawValue)
        if failure == nil { await onChanged() }
    }

    private func stop(_ actuator: Actuator) async {
        failure = await session.actuators.stop(actuator)
        if failure == nil { await onChanged() }
    }

    private func sharedImpact(for actuator: Actuator, alreadyRecorded: Bool) -> String {
        let otherIDs = actuator.plantIDs.filter { $0 != plant.id }
        let verb = alreadyRecorded ? "recorded" : "will record"
        guard !otherIDs.isEmpty else {
            return "Planty \(verb) one airflow entry on \(plant.commonName). "
                + "The fan stops automatically at the selected time."
        }
        let names = otherIDs.compactMap { id in
            session.library.plants.first(where: { $0.id == id })?.commonName
        }
        let others = names.count == otherIDs.count
            ? names.formatted(.list(type: .and))
            : "\(otherIDs.count) other plant\(otherIDs.count == 1 ? "" : "s")"
        return "This fan is shared. Planty \(verb) one airflow entry for each assigned plant: "
            + "\(plant.commonName) and \(others). It stops automatically at the selected time."
    }

    private func timeRemaining(until deadline: Date, now: Date) -> String {
        let seconds = max(0, Int(deadline.timeIntervalSince(now).rounded(.up)))
        let minutes = seconds / 60
        let remainder = seconds % 60
        return seconds == 0 ? "Stopping now…" : "\(minutes):\(String(format: "%02d", remainder)) remaining"
    }
}

struct LightControlSheet: View {
    let plant: Plant

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var selectedActuatorID: UUID?

    private var lights: [Actuator] {
        session.actuators.registered
            .assigned(to: plant.id)
            .filter { $0.kind == .light }
    }

    private var selectedLight: Actuator? {
        lights.first { $0.id == selectedActuatorID } ?? lights.first
    }

    var body: some View {
        NavigationStack {
            Form {
                if lights.count > 1 {
                    Section("Light") {
                        Picker("Light", selection: $selectedActuatorID) {
                            ForEach(lights) { light in
                                Text(light.name).tag(Optional(light.id))
                            }
                        }
                    }
                }

                if let light = selectedLight {
                    LightControlSections(actuator: light)
                        .id(light.id)
                } else {
                    ContentUnavailableView(
                        "No light assigned",
                        systemImage: "lightbulb.slash",
                        description: Text("Register a Home Assistant light in Settings first.")
                    )
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Grow lights")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Done") { dismiss() }
                }
            }
            .task {
                selectedActuatorID = selectedActuatorID ?? lights.first?.id
            }
        }
    }
}
