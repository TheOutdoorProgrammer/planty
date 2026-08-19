import SwiftUI

/// What the link form has gathered so far. `newLink()` is nil until it would
/// be accepted, which is what keeps Save disabled rather than sending nonsense.
struct SensorLinkDraft: Sendable, Equatable {
    var haEntityID = ""
    var role: SensorRole = .soilMoisture
    var target: Target = .plant
    var plantID: UUID?
    var zone = ""

    /// A probe serves one plant; an ambient sensor can speak for a zone. The
    /// service takes exactly one of the two.
    enum Target: String, CaseIterable, Sendable {
        case plant
        case zone
    }

    /// The roles worth offering. `.unknown` exists only to absorb whatever a
    /// future service adds, so the form never proposes it.
    static let offerableRoles: [SensorRole] = [
        .soilMoisture, .ambientTemp, .ambientHumidity, .illuminance
    ]

    var trimmedEntityID: String {
        haEntityID.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// Home Assistant entity ids are `domain.object_id`. Requiring the shape
    /// here catches a pasted friendly name before the service refuses it.
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
            return NewSensorLink(
                plantID: plantID, zone: nil, haEntityID: trimmedEntityID, role: role
            )
        case .zone:
            guard !trimmedZone.isEmpty else { return nil }
            return NewSensorLink(
                plantID: nil, zone: trimmedZone, haEntityID: trimmedEntityID, role: role
            )
        }
    }
}

/// Linking is what lets a Home Assistant entity count as evidence at all, so
/// the sheet spends its room on what the id looks like and who the probe
/// speaks for, not just three fields.
struct LinkSensorSheet: View {
    let api: any PlantyAPI
    let onLinked: (SensorLink) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var draft = SensorLinkDraft()
    @State private var plants: [Plant] = []
    @State private var plantsError: PlantyError?
    @State private var hasLoadedPlants = false
    @State private var isSaving = false
    @State private var failure: PlantyError?

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("sensor.monstera_soil_moisture", text: $draft.haEntityID)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .accessibilityLabel("Home Assistant entity id")
                    Picker("Measures", selection: $draft.role) {
                        ForEach(SensorLinkDraft.offerableRoles, id: \.self) { role in
                            Text(role.label).tag(role)
                        }
                    }
                } header: {
                    Text("Home Assistant entity")
                } footer: {
                    Text("The entity id exactly as Home Assistant knows it, not its friendly name.")
                }

                if !draft.haEntityID.isEmpty && !draft.entityIDLooksRight {
                    Section {
                        Label(
                            "Entity ids look like sensor.monstera_soil_moisture: a domain, a dot, a name, no spaces.",
                            systemImage: "exclamationmark.triangle.fill"
                        )
                        .foregroundStyle(PlantyColor.orange)
                    }
                }

                Section {
                    Picker("Speaks for", selection: $draft.target) {
                        Text("One plant").tag(SensorLinkDraft.Target.plant)
                        Text("A zone").tag(SensorLinkDraft.Target.zone)
                    }
                    .pickerStyle(.segmented)
                    .accessibilityLabel("What the sensor speaks for")

                    switch draft.target {
                    case .plant:
                        plantPicker
                    case .zone:
                        TextField("porch", text: $draft.zone)
                            .textInputAutocapitalization(.never)
                            .autocorrectionDisabled()
                            .accessibilityLabel("Zone name")
                    }
                } footer: {
                    Text("""
                        A probe in a pot serves that plant. An ambient sensor \
                        can serve a zone: one porch thermometer speaks for \
                        every pot on it.
                        """)
                }

                if let failure {
                    Section {
                        Label {
                            Text("""
                                \(failure.errorDescription ?? "The service did not answer.") \
                                Nothing was linked; what you typed is still here.
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
            .navigationTitle("Link a sensor")
            .navigationBarTitleDisplayMode(.inline)
            .interactiveDismissDisabled(isSaving)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .disabled(isSaving)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button(isSaving ? "Linking…" : "Link") {
                        Task { await attemptSave() }
                    }
                    .disabled(draft.newLink() == nil || isSaving)
                }
            }
            .task { await loadPlants() }
        }
    }

    @ViewBuilder
    private var plantPicker: some View {
        if let plantsError {
            Text(plantsError.errorDescription ?? "Could not load the plants.")
                .foregroundStyle(PlantyColor.orange)
        } else if !hasLoadedPlants {
            HStack(spacing: 12) {
                ProgressView()
                Text("Loading plants…")
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        } else if plants.isEmpty {
            Text("No plants yet. Add one first, or link the sensor to a zone.")
                .foregroundStyle(PlantyColor.secondaryText)
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

    /// Dismisses only once the service accepted the link. A failure keeps the
    /// sheet open with everything typed intact.
    private func attemptSave() async {
        guard let link = draft.newLink() else { return }
        isSaving = true
        do {
            let saved = try await api.linkSensor(link)
            isSaving = false
            onLinked(saved)
            dismiss()
        } catch {
            isSaving = false
            failure = PlantyError.from(error)
        }
    }
}
