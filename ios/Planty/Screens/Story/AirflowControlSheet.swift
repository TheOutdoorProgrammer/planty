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
    @State private var start = Date()
    @State private var end = Date()
    @State private var enabled = true
    @State private var failure: PlantyError?
    @State private var confirmsScheduleRemoval = false

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
                    Section("Right now") {
                        HStack {
                            Button("Turn on") { Task { await setState(light, isOn: true) } }
                                .buttonStyle(.borderedProminent)
                                .tint(PlantyColor.green)
                            Button("Turn off") { Task { await setState(light, isOn: false) } }
                                .buttonStyle(.bordered)
                        }
                    }

                    Section {
                        Toggle("Schedule enabled", isOn: $enabled)
                        DatePicker("Turn on", selection: $start, displayedComponents: .hourAndMinute)
                        DatePicker("Turn off", selection: $end, displayedComponents: .hourAndMinute)
                        LabeledContent("Timezone", value: TimeZone.current.identifier)

                        Button("Save schedule") { Task { await save(light) } }
                            .buttonStyle(PrimaryButtonStyle(color: PlantyColor.green))
                    } header: {
                        Text("Daily schedule")
                    } footer: {
                        Text(
                            "Planty checks the schedule every minute and controls only "
                                + "this registered Home Assistant light."
                        )
                    }

                    if let schedule = light.lightSchedule {
                        Section("Last applied") {
                            if let state = schedule.lastAppliedState {
                                LabeledContent("State", value: state ? "On" : "Off")
                            }
                            if let appliedAt = schedule.lastAppliedAt {
                                LabeledContent(
                                    "When",
                                    value: appliedAt.formatted(date: .abbreviated, time: .shortened)
                                )
                            }
                            if let error = schedule.lastError, !error.isEmpty {
                                Text(error).foregroundStyle(PlantyColor.orange)
                            }
                            Button("Remove schedule", role: .destructive) {
                                confirmsScheduleRemoval = true
                            }
                        }
                    }
                } else {
                    ContentUnavailableView(
                        "No light assigned",
                        systemImage: "lightbulb.slash",
                        description: Text("Register a Home Assistant light in Settings first.")
                    )
                }

                if let failure {
                    Section { SheetErrorRow(headline: "The light was not changed.", error: failure) }
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
                loadSchedule()
            }
            .onChange(of: selectedActuatorID) { _, _ in loadSchedule() }
            .confirmationDialog(
                "Remove this light schedule?",
                isPresented: $confirmsScheduleRemoval,
                titleVisibility: .visible
            ) {
                Button("Remove schedule", role: .destructive) {
                    guard let light = selectedLight else { return }
                    Task { failure = await session.actuators.deleteSchedule(light) }
                }
                Button("Keep schedule", role: .cancel) {}
            } message: {
                Text("The light stays registered and can still be controlled manually.")
            }
        }
    }

    private func loadSchedule() {
        let calendar = Calendar.current
        let schedule = selectedLight?.lightSchedule
        enabled = schedule?.enabled ?? true
        let startMinute = schedule?.startMinute ?? 7 * 60
        let endMinute = schedule?.endMinute ?? 21 * 60
        start = calendar.date(
            bySettingHour: startMinute / 60,
            minute: startMinute % 60,
            second: 0,
            of: Date()
        ) ?? Date()
        end = calendar.date(
            bySettingHour: endMinute / 60,
            minute: endMinute % 60,
            second: 0,
            of: Date()
        ) ?? Date()
    }

    private func setState(_ light: Actuator, isOn: Bool) async {
        failure = await session.actuators.setLight(light, isOn: isOn)
    }

    private func save(_ light: Actuator) async {
        let calendar = Calendar.current
        let startParts = calendar.dateComponents([.hour, .minute], from: start)
        let endParts = calendar.dateComponents([.hour, .minute], from: end)
        failure = await session.actuators.setSchedule(
            light,
            startMinute: (startParts.hour ?? 0) * 60 + (startParts.minute ?? 0),
            endMinute: (endParts.hour ?? 0) * 60 + (endParts.minute ?? 0),
            timezone: TimeZone.current.identifier,
            enabled: enabled
        )
    }
}
