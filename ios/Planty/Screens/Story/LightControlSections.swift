import SwiftUI

struct LightControlSections: View {
    let actuatorID: UUID

    @Environment(AppSession.self) private var session
    @State private var failure: PlantyError?
    @State private var isWorking = false

    init(actuator: Actuator) {
        actuatorID = actuator.id
    }

    private var light: Actuator? {
        session.actuators.registered.first { $0.id == actuatorID }
    }

    var body: some View {
        Group {
            if let light {
                Section {
                    HStack(spacing: 10) {
                        Label(light.stateLabel, systemImage: stateIcon(light))
                            .font(.headline)
                            .foregroundStyle(stateColor(light))
                        Spacer()
                        Button("On") { Task { await setState(light, isOn: true) } }
                            .buttonStyle(.borderedProminent)
                            .tint(PlantyColor.green)
                            .disabled(isWorking || light.isOn == true)
                        Button("Off") { Task { await setState(light, isOn: false) } }
                            .buttonStyle(.bordered)
                            .disabled(isWorking || light.isOn == false)
                    }
                } header: {
                    Text("Right now")
                } footer: {
                    Text("This reads and controls the registered Home Assistant light directly.")
                }

                ActuatorScheduleSections(actuator: light)
            }

            if let failure {
                Section { SheetErrorRow(headline: "The light was not changed.", error: failure) }
            }
        }
    }

    private func stateIcon(_ light: Actuator) -> String {
        switch light.isOn {
        case true: "lightbulb.led.fill"
        case false: "lightbulb.led"
        case nil: "lightbulb.slash"
        }
    }

    private func stateColor(_ light: Actuator) -> Color {
        switch light.isOn {
        case true: PlantyColor.yellow
        case false: PlantyColor.secondaryText
        case nil: PlantyColor.orange
        }
    }

    private func setState(_ light: Actuator, isOn: Bool) async {
        guard !isWorking else { return }
        isWorking = true
        defer { isWorking = false }
        failure = await session.actuators.setLight(light, isOn: isOn)
    }
}

struct ActuatorScheduleSections: View {
    let actuatorID: UUID

    @Environment(AppSession.self) private var session
    @State private var draft: ActuatorScheduleDraft
    @State private var failure: PlantyError?
    @State private var isWorking = false
    @State private var confirmsRemoval = false

    init(actuator: Actuator) {
        actuatorID = actuator.id
        _draft = State(initialValue: ActuatorScheduleDraft(schedule: actuator.dailySchedule))
    }

    private var actuator: Actuator? {
        session.actuators.registered.first { $0.id == actuatorID }
    }

    private var schedule: ActuatorSchedule? {
        actuator?.dailySchedule
    }

    private var noun: String {
        actuator?.kind == .fan ? "fan" : "light"
    }

    var body: some View {
        Group {
            if let actuator {
                Section {
                    Toggle("Schedule enabled", isOn: $draft.enabled)
                    DatePicker("Turn on", selection: timeBinding(\.startMinute), displayedComponents: .hourAndMinute)
                    DatePicker("Turn off", selection: timeBinding(\.endMinute), displayedComponents: .hourAndMinute)
                    LabeledContent("Timezone", value: draft.timezone)

                    Button(isWorking ? "Working…" : "Save schedule") {
                        Task { await save(actuator) }
                    }
                    .buttonStyle(PrimaryButtonStyle(color: PlantyColor.green))
                    .disabled(!draft.canSave || isWorking)

                    if schedule != nil {
                        Button("Remove schedule", role: .destructive) {
                            confirmsRemoval = true
                        }
                        .disabled(isWorking)
                    }
                } header: {
                    Text("\(actuator.name) schedule")
                } footer: {
                    Text(scheduleFooter)
                }

                if let schedule,
                   schedule.lastAppliedState != nil || schedule.lastAppliedAt != nil || schedule.lastError != nil {
                    Section("Last schedule run") {
                        if let state = schedule.lastAppliedState {
                            LabeledContent("State", value: state ? "On" : "Off")
                        }
                        if let appliedAt = schedule.lastAppliedAt {
                            LabeledContent(
                                "When",
                                value: appliedAt.formatted(date: .abbreviated, time: .shortened)
                            )
                        }
                        if let error = schedule.lastError?.nilIfBlank {
                            Text(error).foregroundStyle(PlantyColor.orange)
                        }
                    }
                }
            }

            if let failure {
                Section { SheetErrorRow(headline: "The \(noun) schedule was not changed.", error: failure) }
            }
        }
        .confirmationDialog(
            "Remove this \(noun) schedule?",
            isPresented: $confirmsRemoval,
            titleVisibility: .visible
        ) {
            Button("Remove schedule", role: .destructive) {
                guard let actuator else { return }
                Task { await remove(actuator) }
            }
            Button("Keep schedule", role: .cancel) {}
        } message: {
            Text("The \(noun) stays registered and can still be controlled manually.")
        }
    }

    private var scheduleFooter: String {
        let base = "Planty checks this schedule every minute and controls only "
            + "this registered Home Assistant \(noun). Overnight windows are supported."
        if actuator?.kind == .fan {
            return base + " A timed manual run takes priority until it stops."
        }
        return base
    }

    private func timeBinding(_ keyPath: WritableKeyPath<ActuatorScheduleDraft, Int>) -> Binding<Date> {
        Binding(
            get: { date(for: draft[keyPath: keyPath]) },
            set: { date in
                let parts = Calendar.current.dateComponents([.hour, .minute], from: date)
                draft[keyPath: keyPath] = (parts.hour ?? 0) * 60 + (parts.minute ?? 0)
            }
        )
    }

    private func date(for minute: Int) -> Date {
        Calendar.current.date(
            bySettingHour: minute / 60,
            minute: minute % 60,
            second: 0,
            of: Date()
        ) ?? Date()
    }

    private func save(_ actuator: Actuator) async {
        guard !isWorking else { return }
        isWorking = true
        defer { isWorking = false }
        failure = await session.actuators.setSchedule(
            actuator,
            startMinute: draft.startMinute,
            endMinute: draft.endMinute,
            timezone: draft.timezone,
            enabled: draft.enabled
        )
    }

    private func remove(_ actuator: Actuator) async {
        guard !isWorking else { return }
        isWorking = true
        defer { isWorking = false }
        failure = await session.actuators.deleteSchedule(actuator)
    }
}
