import SwiftUI

struct SensorLinkDraft: Sendable, Equatable {
    var haEntityID = ""
    var role: SensorRole = .soilMoisture
    var target: Target = .plant
    var plantID: UUID?
    var zone = ""

    enum Target: String, CaseIterable, Sendable { case plant, zone }

    static let offerableRoles: [SensorRole] = [.soilMoisture, .ambientTemp, .ambientHumidity, .illuminance]

    var trimmedEntityID: String { haEntityID.trimmingCharacters(in: .whitespacesAndNewlines) }
    var entityIDLooksRight: Bool {
        let parts = trimmedEntityID.split(separator: ".")
        return parts.count == 2 && !trimmedEntityID.contains(" ")
    }
    var trimmedZone: String { zone.trimmingCharacters(in: .whitespacesAndNewlines) }

    func newLink() -> NewSensorLink? {
        guard entityIDLooksRight else { return nil }
        switch target {
        case .plant:
            guard let plantID else { return nil }
            return NewSensorLink(plantID: plantID, zone: nil, haEntityID: trimmedEntityID, role: role)
        case .zone:
            guard role != .soilMoisture, !trimmedZone.isEmpty else { return nil }
            return NewSensorLink(plantID: nil, zone: trimmedZone, haEntityID: trimmedEntityID, role: role)
        }
    }
}

private enum EntityEntrySource: String, CaseIterable {
    case discovered
    case custom
    var label: String { self == .discovered ? "Choose" : "Custom ID" }
}

struct LinkSensorSheet: View {
    let api: any PlantyAPI
    let onLinked: (SensorLink) -> Void

    @Environment(\.dismiss) private var dismiss
    @Environment(AppSession.self) private var session
    @State private var draft = SensorLinkDraft()
    @State private var entitySource: EntityEntrySource = .discovered
    @State private var entities: [HomeAssistantEntity] = []
    @State private var entitiesError: PlantyError?
    @State private var hasLoadedEntities = false
    @State private var showingEntityPicker = false
    @State private var plants: [Plant] = []
    @State private var plantsError: PlantyError?
    @State private var hasLoadedPlants = false
    @State private var action = AsyncSheetAction()

    init(
        api: any PlantyAPI,
        plantID: UUID? = nil,
        onLinked: @escaping (SensorLink) -> Void
    ) {
        self.api = api
        self.onLinked = onLinked
        _draft = State(initialValue: SensorLinkDraft(plantID: plantID))
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Picker("Measures", selection: $draft.role) {
                        ForEach(SensorLinkDraft.offerableRoles, id: \.self) { role in Text(role.label).tag(role) }
                    }
                    .onChange(of: draft.role) { _, role in
                        if role == .soilMoisture { draft.target = .plant }
                    }

                    Picker("Entity source", selection: $entitySource) {
                        ForEach(EntityEntrySource.allCases, id: \.self) { source in
                            Text(source.label).tag(source)
                        }
                    }
                    .pickerStyle(.segmented)

                    switch entitySource {
                    case .discovered: discoveredEntityControl
                    case .custom: customEntityControl
                    }
                } header: {
                    Text("Home Assistant entity")
                } footer: {
                    Text(
                        "Planty asks its server for names, rooms and availability; "
                            + "the Home Assistant token never goes to this phone. "
                            + "Use Custom ID for an unusual entity the suggestions omit."
                    )
                }

                if !draft.haEntityID.isEmpty && !draft.entityIDLooksRight {
                    Section {
                        Label(
                            "Entity ids look like sensor.monstera_soil_moisture: "
                                + "a domain, a dot, a name, no spaces.",
                            systemImage: "exclamationmark.triangle.fill"
                        )
                            .foregroundStyle(PlantyColor.orange)
                    }
                }

                Section {
                    if draft.role != .soilMoisture {
                        Picker("Speaks for", selection: $draft.target) {
                            Text("One plant").tag(SensorLinkDraft.Target.plant)
                            Text("A place").tag(SensorLinkDraft.Target.zone)
                        }
                        .pickerStyle(.segmented)
                    }

                    switch draft.target {
                    case .plant: plantPicker
                    case .zone:
                        ManagedChoiceField(
                            title: "Place",
                            emptyLabel: "Choose a place",
                            customLabel: "Add a custom place",
                            choices: session.choices.catalog.places,
                            value: $draft.zone
                        )
                    }
                } footer: {
                    Text(
                        "A probe in a pot serves that plant. An ambient sensor can serve a Place: "
                            + "one porch thermometer speaks for every pot there."
                    )
                }

                if let failure = action.error {
                    Section {
                        Label(
                            "\(failure.errorDescription ?? "The service did not answer.") "
                                + "Nothing was linked; your selection is still here.",
                            systemImage: "exclamationmark.triangle.fill"
                        )
                            .foregroundStyle(PlantyColor.orange)
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Link a sensor")
            .navigationBarTitleDisplayMode(.inline)
            .interactiveDismissDisabled(action.isRunning)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .disabled(action.isRunning)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(action.isRunning ? "Linking…" : "Link") { Task { await attemptSave() } }
                        .disabled(draft.newLink() == nil || action.isRunning)
                }
            }
            .sheet(isPresented: $showingEntityPicker) {
                HomeAssistantEntityPicker(
                    entities: entities,
                    role: draft.role,
                    selection: $draft.haEntityID
                )
            }
            .task { await loadPlants() }
            .task { await loadEntities() }
            .task { await session.choices.loadIfNeeded() }
        }
    }

    private var selectedEntity: HomeAssistantEntity? {
        entities.first { $0.entityID == draft.trimmedEntityID }
    }

    @ViewBuilder private var discoveredEntityControl: some View {
        if let entitiesError {
            Label(
                entitiesError.errorDescription ?? "Could not read Home Assistant entities.",
                systemImage: "exclamationmark.triangle.fill"
            )
                .foregroundStyle(PlantyColor.orange)
            Button("Try discovery again") { Task { await loadEntities() } }
            Button("Enter an entity ID manually") { entitySource = .custom }
        } else if !hasLoadedEntities {
            HStack(spacing: 12) {
                ProgressView()
                Text("Reading Home Assistant entities…")
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        } else if entities.isEmpty {
            Text("No sensor entities were found.").foregroundStyle(PlantyColor.secondaryText)
            Button("Enter an entity ID manually") { entitySource = .custom }
        } else {
            Button { showingEntityPicker = true } label: {
                HStack(spacing: 12) {
                    VStack(alignment: .leading, spacing: 3) {
                        Text(
                            selectedEntity?.friendlyName
                                ?? (draft.trimmedEntityID.isEmpty ? "Choose an entity" : "Custom entity ID")
                        )
                        .foregroundStyle(.primary)
                        if !draft.trimmedEntityID.isEmpty {
                            Text(draft.trimmedEntityID)
                                .font(.caption.monospaced())
                                .foregroundStyle(PlantyColor.secondaryText)
                        }
                    }
                    Spacer()
                    Image(systemName: "chevron.right")
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            }
            .buttonStyle(.plain)
            Button("Enter an entity ID manually") { entitySource = .custom }
        }
    }

    @ViewBuilder private var customEntityControl: some View {
        TextField("sensor.monstera_soil_moisture", text: $draft.haEntityID)
            .textInputAutocapitalization(.never)
            .autocorrectionDisabled()
        if hasLoadedEntities && !entities.isEmpty {
            Button("Choose from Home Assistant") {
                entitySource = .discovered
                showingEntityPicker = true
            }
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
        } else if plants.isEmpty {
            Text("No plants yet. Add one first, or link the sensor to a place.")
                .foregroundStyle(PlantyColor.secondaryText)
        } else {
            Picker("Plant", selection: $draft.plantID) {
                Text("Choose a plant").tag(UUID?.none)
                ForEach(plants) { plant in Text(plant.commonName).tag(Optional(plant.id)) }
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

    private func loadEntities() async {
        hasLoadedEntities = false; entitiesError = nil
        do {
            entities = try await api.homeAssistantEntities()
        } catch {
            guard !PlantyError.isCancellation(error) else { return }
            entitiesError = PlantyError.from(error)
        }
        hasLoadedEntities = true
    }

    private func attemptSave() async {
        guard let link = draft.newLink() else { return }
        guard let saved = await action.performThrowing({ try await api.linkSensor(link) }) else { return }
        await session.choices.load()
        onLinked(saved)
        dismiss()
    }
}

private struct HomeAssistantEntityPicker: View {
    let entities: [HomeAssistantEntity]
    let role: SensorRole
    @Binding var selection: String
    @Environment(\.dismiss) private var dismiss
    @State private var search = ""
    @State private var showAll = false

    var body: some View {
        NavigationStack {
            List {
                if !likelyMatches.isEmpty {
                    Section("\(role.label) suggestions") {
                        ForEach(likelyMatches) { entity in entityRow(entity) }
                    }
                }
                if shouldShowOtherMatches && !otherMatches.isEmpty {
                    Section("Other sensor entities") {
                        ForEach(otherMatches) { entity in entityRow(entity) }
                    }
                }
                if likelyMatches.isEmpty && (!shouldShowOtherMatches || otherMatches.isEmpty) {
                    Section {
                        VStack(alignment: .leading, spacing: 8) {
                            Text(
                                search.isEmpty
                                    ? "No likely \(role.label.lowercased()) entities"
                                    : "No matching entities"
                            )
                            .font(.headline)
                            Text("Try another search, show every sensor, or return and use Custom ID.")
                                .font(.subheadline).foregroundStyle(PlantyColor.secondaryText)
                            if !showAll { Button("Show all sensors") { showAll = true } }
                        }
                    }
                }
            }
            .navigationTitle("Choose an entity")
            .navigationBarTitleDisplayMode(.inline)
            .searchable(text: $search, prompt: "Name, entity ID, or area")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .primaryAction) {
                    Button(showAll ? "Suggestions" : "All sensors") { showAll.toggle() }
                }
            }
        }
    }

    private var allMatches: [HomeAssistantEntity] {
        HomeAssistantEntityFilter.all(in: entities, matching: search)
    }
    private var likelyMatches: [HomeAssistantEntity] {
        HomeAssistantEntityFilter.likely(in: entities, for: role, matching: search)
    }
    private var otherMatches: [HomeAssistantEntity] {
        let likelyIDs = Set(likelyMatches.map(\.entityID))
        return allMatches.filter { !likelyIDs.contains($0.entityID) }
    }
    private var shouldShowOtherMatches: Bool {
        showAll || !search.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    }

    private func entityRow(_ entity: HomeAssistantEntity) -> some View {
        Button { selection = entity.entityID; dismiss() } label: {
            HStack(alignment: .top, spacing: 12) {
                VStack(alignment: .leading, spacing: 4) {
                    HStack(spacing: 8) {
                        Text(entity.friendlyName)
                            .font(.body.weight(.medium))
                            .foregroundStyle(.primary)
                        if !entity.available {
                            Text("Unavailable")
                                .font(.caption2.weight(.semibold))
                                .foregroundStyle(PlantyColor.orange)
                        }
                    }
                    Text(entity.entityID)
                        .font(.caption.monospaced())
                        .foregroundStyle(PlantyColor.secondaryText)
                    if let metadata = entity.metadataLabel {
                        Text(metadata)
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                }
                Spacer()
                if selection == entity.entityID {
                    Image(systemName: "checkmark").font(.body.weight(.semibold))
                }
            }
        }
        .buttonStyle(.plain)
    }
}
