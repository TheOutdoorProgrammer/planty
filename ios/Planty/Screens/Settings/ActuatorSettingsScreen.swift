import SwiftUI

struct ActuatorSettingsScreen: View {
    @Environment(AppSession.self) private var session
    @State private var isRegistering = false

    var body: some View {
        List {
            Section {
                Label("Recurring schedules stay in Home Assistant", systemImage: "house.and.flag.fill")
                    .font(.headline)
                    .foregroundStyle(PlantyColor.cyan)
                Text(
                    "Planty can start a registered fan or switch for a bounded lease and stop it " +
                    "by Planty actuator ID. It does not create or own recurring schedules."
                )
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            Section("Registered actuators") {
                if let error = session.actuators.error {
                    Text(error.errorDescription ?? "Actuators could not be loaded.")
                        .foregroundStyle(PlantyColor.orange)
                }
                if !session.actuators.hasLoaded {
                    HStack { ProgressView(); Text("Loading registered actuators…") }
                } else if session.actuators.registered.isEmpty {
                    Text("No fan or switch has been explicitly registered.")
                        .foregroundStyle(PlantyColor.secondaryText)
                }
                ForEach(session.actuators.registered) { actuator in
                    NavigationLink {
                        ActuatorDetailScreen(actuatorID: actuator.id)
                    } label: {
                        ActuatorSummaryRow(
                            actuator: actuator,
                            lease: session.actuators.leases[actuator.id]
                        )
                    }
                }
                Button("Register a discovered entity") { isRegistering = true }
            }
        }
        .scrollContentBackground(.hidden)
        .plantyPage()
        .navigationTitle("Fans and switches")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await session.actuators.load() }
        .task { await session.actuators.load() }
        .sheet(isPresented: $isRegistering) { ActuatorRegistrationSheet() }
    }
}

private struct ActuatorSummaryRow: View {
    let actuator: Actuator
    let lease: ActuatorLease?

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Label(actuator.name, systemImage: actuator.kind.symbol)
                .font(.body.weight(.semibold))
            Text(actuator.entityID)
                .font(.caption.monospaced())
                .foregroundStyle(PlantyColor.secondaryText)
            if let lease, lease.isActive {
                Text("Lease ends \(lease.deadline.formatted(date: .omitted, time: .shortened))")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(PlantyColor.green)
            }
        }
        .accessibilityElement(children: .combine)
    }
}

private struct ActuatorRegistrationSheet: View {
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var search = ""
    @State private var selected: HomeAssistantEntity?
    @State private var selectedPlantIDs: Set<UUID> = []
    @State private var name = ""
    @State private var failure: PlantyError?

    private var matches: [HomeAssistantEntity] {
        session.actuators.discovered.filter { $0.matches(search: search) }
    }

    var body: some View {
        NavigationStack {
            List {
                Section {
                    Text(
                        "Choose the exact Home Assistant fan or switch. Planty will never guess " +
                        "which plug controls a plant device."
                    )
                        .font(.subheadline)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
                if let failure {
                    Section { SheetErrorRow(headline: "Nothing was registered.", error: failure) }
                }
                if let selected {
                    Section("Registration name") {
                        TextField("Name", text: $name)
                        LabeledContent("Entity", value: selected.entityID)
                    }
                }
                Section("Plants served") {
                    if session.library.plants.isEmpty {
                        Text("No living plants are available.")
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                    ForEach(session.library.plants.filter { !$0.status.isRetired }) { plant in
                        Button {
                            if selectedPlantIDs.contains(plant.id) {
                                selectedPlantIDs.remove(plant.id)
                            } else {
                                selectedPlantIDs.insert(plant.id)
                            }
                        } label: {
                            HStack {
                                VStack(alignment: .leading) {
                                    Text(plant.commonName).foregroundStyle(.primary)
                                    Text(plant.haArea ?? plant.location)
                                        .font(.caption)
                                        .foregroundStyle(PlantyColor.secondaryText)
                                }
                                Spacer()
                                Image(
                                    systemName: selectedPlantIDs.contains(plant.id)
                                        ? "checkmark.circle.fill" : "circle"
                                )
                                .foregroundStyle(
                                    selectedPlantIDs.contains(plant.id)
                                        ? PlantyColor.green : PlantyColor.secondaryText
                                )
                            }
                        }
                        .buttonStyle(.plain)
                    }
                }
                Section("Discovered fans and switches") {
                    if session.actuators.isDiscovering {
                        HStack { ProgressView(); Text("Asking Home Assistant…") }
                    } else if matches.isEmpty {
                        Text(
                            search.isEmpty
                                ? "No fan or switch entities were discovered."
                                : "No matching entities."
                        )
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                    ForEach(matches) { entity in
                        Button {
                            selected = entity
                            name = entity.friendlyName
                        } label: {
                            HStack(alignment: .top, spacing: 10) {
                                VStack(alignment: .leading, spacing: 3) {
                                    Text(entity.friendlyName).foregroundStyle(.primary)
                                    Text(entity.entityID)
                                        .font(.caption.monospaced())
                                        .foregroundStyle(PlantyColor.secondaryText)
                                    if let area = entity.area {
                                        Text(area).font(.caption).foregroundStyle(PlantyColor.secondaryText)
                                    }
                                    if !entity.available {
                                        Text("Currently unavailable")
                                            .font(.caption)
                                            .foregroundStyle(PlantyColor.orange)
                                    }
                                }
                                Spacer()
                                if selected?.id == entity.id {
                                    Image(systemName: "checkmark.circle.fill")
                                        .foregroundStyle(PlantyColor.green)
                                }
                            }
                        }
                        .buttonStyle(.plain)
                        .accessibilityValue(selected?.id == entity.id ? "Selected" : "Not selected")
                    }
                }
            }
            .searchable(text: $search, prompt: "Name, entity ID, or area")
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Register actuator")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Register") { Task { await register() } }
                        .disabled(selected == nil || name.cleaned.isEmpty || selectedPlantIDs.isEmpty)
                }
            }
            .task {
                async let discovery: Void = session.actuators.discover()
                async let plants: Void = session.library.load()
                _ = await (discovery, plants)
            }
        }
    }

    private func register() async {
        guard let selected else { return }
        failure = await session.actuators.register(
            entity: selected,
            name: name,
            plantIDs: Array(selectedPlantIDs)
        )
        if failure == nil { dismiss() }
    }
}

private struct ActuatorDetailScreen: View {
    let actuatorID: UUID

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var duration = ActuatorRunDuration.tenMinutes
    @State private var name = ""
    @State private var selectedPlantIDs: Set<UUID> = []
    @State private var policyControlEnabled = false
    @State private var failure: PlantyError?
    @State private var confirmsRemoval = false

    private var actuator: Actuator? { session.actuators.registered.first { $0.id == actuatorID } }
    private var lease: ActuatorLease? { session.actuators.leases[actuatorID] }
    private var events: [ActuatorEvent] { session.actuators.events[actuatorID] ?? [] }

    var body: some View {
        Form {
            if let actuator {
                Section {
                    TextField("Name", text: $name)
                    LabeledContent("Home Assistant entity", value: actuator.entityID)
                    LabeledContent("Kind", value: actuator.kind.label)
                    ForEach(session.library.plants.filter { !$0.status.isRetired }) { plant in
                        Toggle(plant.commonName, isOn: plantBinding(plant.id))
                    }
                    if actuator.kind == .fan {
                        Toggle("Allow enforcing policies", isOn: $policyControlEnabled)
                    }
                    Button("Save registration") { Task { await save(actuator) } }
                        .disabled(
                            name.cleaned.isEmpty || selectedPlantIDs.isEmpty ||
                            (name.cleaned == actuator.name &&
                             selectedPlantIDs == Set(actuator.plantIDs) &&
                             policyControlEnabled == actuator.policyControlEnabled)
                        )
                } header: {
                    Text("Registration")
                } footer: {
                    Text(
                        "Policy control is a separate opt-in and only works for fans. " +
                        "Every run is still bounded by a durable lease."
                    )
                }

                if actuator.kind != .light {
                    Section {
                    if let lease, lease.isActive {
                        LabeledContent("Current lease", value: "Running")
                        LabeledContent("Deadline") {
                            Text(lease.deadline.formatted(date: .abbreviated, time: .shortened))
                        }
                        LabeledContent("Requested by", value: lease.actor)
                    } else {
                        Text("No active lease is known.").foregroundStyle(PlantyColor.secondaryText)
                    }
                    Picker("Bounded run time", selection: $duration) {
                        ForEach(ActuatorRunDuration.allCases) { duration in
                            Text(duration.label).tag(duration)
                        }
                    }
                    Button("Start bounded run") {
                        Task {
                            failure = await session.actuators.start(
                                actuator,
                                durationSeconds: duration.rawValue
                            )
                        }
                    }
                        .disabled(session.actuators.controlling.contains(actuator.id) || lease?.isActive == true)
                    Button("Stop now") { Task { failure = await session.actuators.stop(actuator) } }
                        .disabled(session.actuators.controlling.contains(actuator.id))
                    } header: {
                        Text("Manual control")
                    } footer: {
                    Text(
                        "Every start has a deadline. Stop is safe to repeat and addresses this " +
                        "Planty actuator ID. Recurring schedules remain Home Assistant-owned."
                    )
                    }
                }

                if let failure {
                    Section { SheetErrorRow(headline: "The actuator was not changed.", error: failure) }
                }

                Section("Audit history") {
                    if events.isEmpty {
                        Text("No actuator events recorded.").foregroundStyle(PlantyColor.secondaryText)
                    }
                    ForEach(events) { event in ActuatorEventRow(event: event) }
                }

                Section {
                    Button("Remove registration", role: .destructive) { confirmsRemoval = true }
                        .disabled(lease?.isActive == true)
                } footer: {
                    Text(
                        "Stop an active lease before removing the registration. " +
                        "The Home Assistant entity itself is not deleted."
                    )
                }
            }
        }
        .scrollContentBackground(.hidden)
        .plantyPage()
        .navigationTitle(actuator?.name ?? "Actuator")
        .navigationBarTitleDisplayMode(.inline)
        .onAppear {
            name = actuator?.name ?? ""
            selectedPlantIDs = Set(actuator?.plantIDs ?? [])
            policyControlEnabled = actuator?.policyControlEnabled ?? false
        }
        .task {
            async let plants: Void = session.library.load()
            if let actuator { await session.actuators.loadEvents(for: actuator) }
            _ = await plants
        }
        .confirmationDialog(
            "Remove this Planty registration?",
            isPresented: $confirmsRemoval,
            titleVisibility: .visible
        ) {
            Button("Remove registration", role: .destructive) { Task { await remove() } }
            Button("Keep registration", role: .cancel) {}
        } message: {
            Text("This does not delete or modify the Home Assistant entity.")
        }
    }

    private func save(_ actuator: Actuator) async {
        failure = await session.actuators.update(
            actuator,
            name: name,
            plantIDs: Array(selectedPlantIDs),
            policyControlEnabled: policyControlEnabled
        )
    }

    private func plantBinding(_ id: UUID) -> Binding<Bool> {
        Binding(
            get: { selectedPlantIDs.contains(id) },
            set: { selected in
                if selected { selectedPlantIDs.insert(id) } else { selectedPlantIDs.remove(id) }
            }
        )
    }

    private func remove() async {
        guard let actuator else { return }
        failure = await session.actuators.remove(actuator)
        if failure == nil { dismiss() }
    }
}

private struct ActuatorEventRow: View {
    let event: ActuatorEvent

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(event.action.label).font(.subheadline.weight(.semibold))
            if let detail = event.detail?.nilIfBlank {
                Text(detail).font(.caption).foregroundStyle(PlantyColor.secondaryText)
            }
            Text(
                "\(event.source.label) · \(event.actor) · " +
                event.createdAt.formatted(date: .abbreviated, time: .shortened)
            )
                .font(.caption2)
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .accessibilityElement(children: .combine)
    }
}

private extension ActuatorKind {
    var label: String {
        switch self {
        case .fan: "Fan"
        case .switch: "Switch"
        case .light: "Light"
        case .unknown: "Unknown"
        }
    }
    var symbol: String {
        switch self {
        case .fan: "fan.fill"
        case .switch: "powerplug.fill"
        case .light: "lightbulb.led.fill"
        case .unknown: "questionmark.circle"
        }
    }
}

private extension ActuatorEventAction {
    var label: String { rawValue.replacingOccurrences(of: "_", with: " ").capitalized }
}

private extension ObservationSource {
    var label: String { rawValue.capitalized }
}
