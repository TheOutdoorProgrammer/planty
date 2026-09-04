import SwiftUI

struct SensorAssignmentDraft: Equatable {
    var target: SensorLinkDraft.Target
    var plantID: UUID?
    var zone: String

    init(link: SensorLink) {
        target = link.plantID == nil ? .zone : .plant
        plantID = link.plantID
        zone = link.zone ?? ""
    }

    var trimmedZone: String { zone.trimmingCharacters(in: .whitespacesAndNewlines) }

    func assignment(for role: SensorRole) -> SensorAssignment? {
        switch target {
        case .plant:
            guard let plantID else { return nil }
            return SensorAssignment(plantID: plantID, zone: nil)
        case .zone:
            guard role != .soilMoisture, !trimmedZone.isEmpty else { return nil }
            return SensorAssignment(plantID: nil, zone: trimmedZone)
        }
    }
}

struct SensorAssignmentSheet: View {
    let api: any PlantyAPI
    let onSaved: (SensorSeries) -> Void

    @Environment(\.dismiss) private var dismiss
    @Environment(AppSession.self) private var session
    @State private var series: SensorSeries
    @State private var draft: SensorAssignmentDraft
    @State private var plants: [Plant] = []
    @State private var hasLoadedPlants = false
    @State private var plantsError: PlantyError?
    @State private var action = AsyncSheetAction()
    @State private var isCalibrating = false

    init(api: any PlantyAPI, sensor: SensorSeries, onSaved: @escaping (SensorSeries) -> Void) {
        self.api = api
        self.onSaved = onSaved
        _series = State(initialValue: sensor)
        _draft = State(initialValue: SensorAssignmentDraft(link: sensor.link))
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Text(series.link.haEntityID)
                        .font(.headline)
                    LabeledContent("Measures", value: series.link.role.label)
                } header: {
                    Text("Home Assistant entity")
                } footer: {
                    Text(
                        "The entity and measurement stay with this sensor. "
                            + "Its plant or place can change without losing readings."
                    )
                }

                Section {
                    if series.link.role != .soilMoisture {
                        Picker("Speaks for", selection: $draft.target) {
                            Text("One plant").tag(SensorLinkDraft.Target.plant)
                            Text("A place").tag(SensorLinkDraft.Target.zone)
                        }
                        .pickerStyle(.segmented)
                    }

                    switch draft.target {
                    case .plant:
                        plantPicker
                    case .zone:
                        ManagedChoiceField(
                            title: "Place",
                            emptyLabel: "Choose a place",
                            customLabel: "Add a custom place",
                            choices: session.choices.catalog.places,
                            value: $draft.zone
                        )
                    }
                } header: {
                    Text("Assignment")
                } footer: {
                    Text(
                        "A probe in a pot serves one plant. "
                            + "An ambient sensor can instead serve every plant in a place."
                    )
                }

                if series.link.role.requiresCalibration {
                    Section {
                        Button(series.link.isCalibrated ? "Change calibration" : "Calibrate probe") {
                            isCalibrating = true
                        }
                    } header: {
                        Text("Moisture calibration")
                    } footer: {
                        Text(
                            series.link.isCalibrated
                                ? "The probe keeps its dry and wet baselines when it moves to another plant."
                                : "Until calibrated, Planty reports the raw value "
                                    + "but never uses it to decide when to water."
                        )
                    }
                }

                if let failure = action.error {
                    Section {
                        SheetErrorRow(headline: "The assignment was not changed.", error: failure)
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Sensor settings")
            .navigationBarTitleDisplayMode(.inline)
            .interactiveDismissDisabled(action.isRunning)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }.disabled(action.isRunning)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(action.isRunning ? "Saving…" : "Save") {
                        Task { await saveAssignment() }
                    }
                    .disabled(draft.assignment(for: series.link.role) == nil || action.isRunning)
                }
            }
            .sheet(isPresented: $isCalibrating) {
                CalibrateSensorSheet(link: series.link, latest: series.latest) { calibration in
                    await saveCalibration(calibration)
                }
            }
            .task { await loadPlants() }
            .task { await session.choices.loadIfNeeded() }
        }
    }

    @ViewBuilder private var plantPicker: some View {
        if let plantsError {
            Text(plantsError.errorDescription ?? "Could not load the plants.")
                .foregroundStyle(PlantyColor.orange)
        } else if !hasLoadedPlants {
            HStack(spacing: 12) {
                ProgressView()
                Text("Loading plants…").foregroundStyle(PlantyColor.secondaryText)
            }
        } else {
            Picker("Plant", selection: $draft.plantID) {
                Text("Choose a plant").tag(UUID?.none)
                ForEach(plants) { plant in
                    Text(plant.commonName).tag(Optional(plant.id))
                }
            }
        }
    }

    private func loadPlants() async {
        do {
            plants = try await api.plants(filter: .live)
            plantsError = nil
        } catch {
            guard !PlantyError.isCancellation(error) else { return }
            plantsError = PlantyError.from(error)
        }
        hasLoadedPlants = true
    }

    private func saveAssignment() async {
        guard let assignment = draft.assignment(for: series.link.role) else { return }
        guard let saved = await action.performThrowing({
            try await api.assignSensor(id: series.id, to: assignment)
        }) else { return }
        series = SensorSeries(link: saved, readings: series.readings)
        onSaved(series)
        dismiss()
    }

    private func saveCalibration(_ calibration: SensorCalibration) async -> PlantyError? {
        do {
            let saved = try await api.calibrate(sensorID: series.id, to: calibration)
            series = SensorSeries(link: saved, readings: series.readings)
            onSaved(series)
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }
}

struct PlantSensorConnectionsScreen: View {
    let api: any PlantyAPI
    let plant: Plant
    let onChanged: () -> Void

    @State private var sensors: [SensorSeries]
    @State private var hasLoaded = false
    @State private var error: PlantyError?
    @State private var editing: SensorSeries?
    @State private var isLinking = false

    init(
        api: any PlantyAPI,
        plant: Plant,
        sensors: [SensorSeries],
        onChanged: @escaping () -> Void
    ) {
        self.api = api
        self.plant = plant
        self.onChanged = onChanged
        _sensors = State(initialValue: sensors.filter { $0.link.plantID == plant.id })
    }

    var body: some View {
        List {
            if let error {
                Text(error.errorDescription ?? "Could not load sensors.")
                    .foregroundStyle(PlantyColor.orange)
            }
            if !hasLoaded && sensors.isEmpty {
                HStack(spacing: 12) {
                    ProgressView()
                    Text("Reading sensor assignments…")
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            } else if sensors.isEmpty && error == nil {
                VStack(alignment: .leading, spacing: 8) {
                    Text("No sensors serve this plant.").font(.headline)
                    Text("Link a Home Assistant sensor or move an existing one here.")
                        .font(.subheadline)
                        .foregroundStyle(PlantyColor.secondaryText)
                    Button("Link a sensor") { isLinking = true }
                        .buttonStyle(SecondaryButtonStyle())
                }
            }
            ForEach(sensors) { sensor in
                Button { editing = sensor } label: {
                    VStack(alignment: .leading, spacing: 4) {
                        Text(sensor.link.role.label).font(.headline)
                        Text(sensor.link.haEntityID)
                            .font(.caption.monospaced())
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                .accessibilityHint("Opens sensor settings.")
            }
        }
        .scrollContentBackground(.hidden)
        .plantyPage()
        .navigationTitle("Sensor connections")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button { isLinking = true } label: {
                    Label("Link a sensor", systemImage: "plus")
                }
            }
        }
        .sheet(item: $editing) { sensor in
            SensorAssignmentSheet(api: api, sensor: sensor, onSaved: apply)
        }
        .sheet(isPresented: $isLinking) {
            LinkSensorSheet(api: api, plantID: plant.id) { _ in
                Task {
                    await load()
                    onChanged()
                }
            }
        }
        .task { await load() }
    }

    private func load() async {
        do {
            sensors = try await api.sensors().filter { $0.link.plantID == plant.id }
            error = nil
        } catch {
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
        hasLoaded = true
    }

    private func apply(_ saved: SensorSeries) {
        sensors.removeAll { $0.id == saved.id }
        if saved.link.plantID == plant.id { sensors.append(saved) }
        sensors.sort { $0.link.role.label < $1.link.role.label }
        onChanged()
    }
}
