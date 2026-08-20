import SwiftUI

/// What the edit sheet holds while somebody types, and the diffing that turns
/// it into a sparse patch. Pure so the "send only what changed" promise is
/// testable without a screen.
struct PlantEditForm {
    var commonName: String
    var botanicalName: String
    var location: String
    var haArea: String
    var steward: String
    var status: PlantStatus
    var lightExposure: LightExposure
    var minTempText: String
    var wateringMethod: WateringMethod
    var accessibility: PlantAccessibility
    var potSizeText: String
    var potMaterial: String
    var drainage: DrainageChoice

    /// Not part of the patch: shelter is its own endpoint, since it is a fact
    /// about tonight rather than about the plant.
    var sheltered: Bool

    enum DrainageChoice: Hashable {
        case unrecorded
        case drains
        case sealed
    }

    enum EditIssue: Error, Equatable {
        case notANumber(field: String, entered: String)

        var message: String {
            switch self {
            case .notANumber(let field, let entered):
                "Planty cannot read “\(entered)” as \(field). Digits only, like 42 or 6.5."
            }
        }
    }

    init(plant: Plant) {
        commonName = plant.commonName
        botanicalName = plant.botanicalName ?? ""
        location = plant.haArea?.cleaned.isEmpty == false ? plant.haArea! : plant.location
        haArea = plant.haArea ?? ""
        steward = plant.steward
        status = plant.status
        lightExposure = plant.lightExposure ?? .unknown
        minTempText = Self.text(plant.minTempF)
        wateringMethod = plant.wateringMethod
        accessibility = plant.accessibility
        potSizeText = Self.text(plant.potSizeIn)
        potMaterial = plant.potMaterial ?? ""
        sheltered = plant.isSheltered
        drainage = switch plant.hasDrainage {
        case true?: .drains
        case false?: .sealed
        case nil: .unrecorded
        }
    }

    /// Only differences from `plant` land in the patch, so an untouched form
    /// produces an empty one and nothing goes on the wire.
    func patch(against plant: Plant) -> Result<PlantPatch, EditIssue> {
        var patch = PlantPatch()
        applyNames(to: &patch, against: plant)
        applyPlacement(to: &patch, against: plant)
        applyChoices(to: &patch, against: plant)
        if let issue = applyNumbers(to: &patch, against: plant) {
            return .failure(issue)
        }
        applyPot(to: &patch, against: plant)
        return .success(patch)
    }

    private func applyNames(to patch: inout PlantPatch, against plant: Plant) {
        let name = commonName.cleaned
        if !name.isEmpty, name != plant.commonName { patch.commonName = name }
        setIfChanged(&patch.botanicalName, to: botanicalName, from: plant.botanicalName)
    }

    // Existing records may already have a room and HA area that differ. An
    // untouched form preserves that legacy data. Selecting a new Place in the
    // UI updates both fields, so new edits cannot create more drift.
    private func applyPlacement(to patch: inout PlantPatch, against plant: Plant) {
        let room = location.cleaned
        let originalPlace = (plant.haArea?.cleaned.isEmpty == false ? plant.haArea! : plant.location).cleaned
        if !room.isEmpty, room != originalPlace {
            patch.location = room
            patch.haArea = room
        }
        let keeper = steward.cleaned
        if !keeper.isEmpty, keeper != plant.steward { patch.steward = keeper }
    }

    private func applyChoices(to patch: inout PlantPatch, against plant: Plant) {
        if status != plant.status { patch.status = status }
        if lightExposure != (plant.lightExposure ?? .unknown) { patch.lightExposure = lightExposure }
        if wateringMethod != plant.wateringMethod { patch.wateringMethod = wateringMethod }
        if accessibility != plant.accessibility { patch.accessibility = accessibility }
    }

    private func applyNumbers(to patch: inout PlantPatch, against plant: Plant) -> EditIssue? {
        switch number(from: minTempText, field: "the cold limit") {
        case .failure(let issue):
            return issue
        case .success(let limit):
            if let limit, limit != plant.minTempF { patch.minTempF = limit }
        }
        switch number(from: potSizeText, field: "the pot size") {
        case .failure(let issue):
            return issue
        case .success(let size):
            if let size, size != plant.potSizeIn { patch.potSizeIn = size }
        }
        return nil
    }

    private func applyPot(to patch: inout PlantPatch, against plant: Plant) {
        setIfChanged(&patch.potMaterial, to: potMaterial, from: plant.potMaterial)
        let drains: Bool? = switch drainage {
        case .unrecorded: nil
        case .drains: true
        case .sealed: false
        }
        if let drains, drains != plant.hasDrainage { patch.hasDrainage = drains }
    }

    /// An emptied field clears a value the record already has; one that
    /// started empty and stayed empty is not a change.
    private func setIfChanged(_ slot: inout String?, to text: String, from original: String?) {
        let trimmed = text.cleaned
        guard trimmed != (original ?? "") else { return }
        slot = trimmed
    }

    /// Empty means "left alone". A comma decimal is accepted because that is
    /// what half the world's keyboards produce.
    private func number(from text: String, field: String) -> Result<Double?, EditIssue> {
        let trimmed = text.cleaned
        guard !trimmed.isEmpty else { return .success(nil) }
        guard let value = Double(trimmed.replacingOccurrences(of: ",", with: ".")) else {
            return .failure(.notANumber(field: field, entered: trimmed))
        }
        return .success(value)
    }

    private static func text(_ value: Double?) -> String {
        guard let value else { return "" }
        return value.formatted(.number.grouping(.never))
    }
}

/// Every field a wrong first impression can get wrong, correctable at last.
/// Closes only on success; a failure keeps the sheet and the typing.
struct EditPlantSheet: View {
    let plant: Plant
    let choices: ManagedChoicesStore
    let save: (PlantPatch) async -> PlantyError?

    /// Separate because shelter has its own endpoint, and moving it here is
    /// what stopped it being tapped by accident on the plant's own page.
    let setSheltered: (Bool) async -> PlantyError?

    @Environment(\.dismiss) private var dismiss
    @State private var form: PlantEditForm
    @State private var error: PlantyError?
    @State private var validation: String?
    @State private var isSaving = false

    init(
        plant: Plant,
        choices: ManagedChoicesStore,
        save: @escaping (PlantPatch) async -> PlantyError?,
        setSheltered: @escaping (Bool) async -> PlantyError? = { _ in nil }
    ) {
        self.plant = plant
        self.choices = choices
        self.save = save
        self.setSheltered = setSheltered
        _form = State(initialValue: PlantEditForm(plant: plant))
    }

    var body: some View {
        NavigationStack {
            Form {
                if let error {
                    Section {
                        SheetErrorRow(
                            headline: "Not saved. Your changes are still here.",
                            error: error
                        )
                    }
                }
                if let validation {
                    Section {
                        Label(validation, systemImage: "exclamationmark.triangle.fill")
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(PlantyColor.orange)
                            .listRowBackground(PlantyColor.orange.opacity(0.12))
                    }
                }

                Section("What is it") {
                    TextField("Common name", text: $form.commonName)
                    TextField("Botanical name", text: $form.botanicalName)
                }

                Section {
                    ManagedChoiceField(
                        title: "Place",
                        emptyLabel: "Choose a place",
                        customLabel: "Add a custom place",
                        choices: choices.catalog.places,
                        value: $form.location
                    )
                } header: {
                    Text("Where it lives")
                } footer: {
                    Text("One Place is shared with Home Assistant areas and ambient sensor zones.")
                }

                Section {
                    ManagedChoiceField(
                        title: "Owner",
                        emptyLabel: "Choose an owner",
                        customLabel: "Add a custom owner",
                        choices: choices.catalog.owners,
                        value: $form.steward
                    )
                } header: {
                    Text("Whose is it")
                } footer: {
                    Text("“\(Plant.stewardSelf)” means it is yours.")
                }

                if !plant.status.isRetired {
                    Section("How it is doing") {
                        Picker("Status", selection: $form.status) {
                            ForEach(statusOptions, id: \.self) { Text($0.editLabel).tag($0) }
                        }
                    }
                }

                Section {
                    Picker("Light", selection: $form.lightExposure) {
                        ForEach(lightOptions, id: \.self) { Text($0.label).tag($0) }
                    }
                    LabeledContent("Cold limit °F") {
                        TextField("none", text: $form.minTempText)
                            .keyboardType(.numbersAndPunctuation)
                            .multilineTextAlignment(.trailing)
                            .accessibilityLabel("Cold limit in degrees Fahrenheit")
                    }
                } header: {
                    Text("Light and cold")
                } footer: {
                    Text("Below the cold limit, Planty asks for this plant to be brought indoors.")
                }

                if plant.canShelter {
                    Section {
                        Toggle("Indoors right now", isOn: $form.sheltered)
                    } header: {
                        Text("Where it is")
                    } footer: {
                        Text("""
                            Only for a plant that lives outside and was carried \
                            in ahead of a cold night. Planty stops asking while \
                            this is on, and says when it can go back out.
                            """)
                    }
                }

                Section("Watering and reach") {
                    Picker("Watering", selection: $form.wateringMethod) {
                        ForEach(wateringOptions, id: \.self) { Text($0.label).tag($0) }
                    }
                    Picker("How hard to reach", selection: $form.accessibility) {
                        ForEach(accessibilityOptions, id: \.self) { Text($0.editLabel).tag($0) }
                    }
                }

                Section("The pot") {
                    LabeledContent("Size, inches") {
                        TextField("none", text: $form.potSizeText)
                            .keyboardType(.decimalPad)
                            .multilineTextAlignment(.trailing)
                            .accessibilityLabel("Pot size in inches")
                    }
                    ManagedChoiceField(
                        title: "Material",
                        emptyLabel: "Not recorded",
                        customLabel: "Add a custom material",
                        choices: choices.catalog.potMaterials,
                        value: $form.potMaterial
                    )
                    Picker("Drainage", selection: $form.drainage) {
                        ForEach(drainageOptions, id: \.self) { Text(drainageLabel($0)).tag($0) }
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Edit \(plant.commonName)")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .disabled(isSaving)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button {
                        Task { await submit() }
                    } label: {
                        if isSaving {
                            ProgressView()
                        } else {
                            Text("Save")
                        }
                    }
                    .disabled(isSaving || form.commonName.cleaned.isEmpty)
                    .accessibilityLabel(isSaving ? "Saving" : "Save changes")
                }
            }
            .task { await choices.loadIfNeeded() }
        }
        .interactiveDismissDisabled(isSaving)
    }

    private func submit() async {
        validation = nil
        guard case .success(let patch) = form.patch(against: plant) else {
            if case .failure(let issue) = form.patch(against: plant) {
                validation = issue.message
            }
            return
        }

        let shelterChanged = form.sheltered != plant.isSheltered
        if patch.isEmpty, !shelterChanged {
            dismiss()
            return
        }

        isSaving = true
        defer { isSaving = false }

        if !patch.isEmpty {
            error = await save(patch)
            if error != nil { return }
        }
        // Its own endpoint, so a failure here must not read as the edit having
        // failed when the edit already landed.
        if shelterChanged {
            error = await setSheltered(form.sheltered)
            if error != nil { return }
        }
        await choices.load()
        dismiss()
    }

    // Unknown never appears as a choice; it only stays visible when it is what
    // the record already says, so the picker cannot lie about the start state.
    private var statusOptions: [PlantStatus] {
        withCurrent([.alive, .struggling, .dormant], current: plant.status)
    }

    private var lightOptions: [LightExposure] {
        withCurrent(
            LightExposure.allCases.filter { $0 != .unknown },
            current: plant.lightExposure ?? .unknown
        )
    }

    private var wateringOptions: [WateringMethod] {
        withCurrent(
            WateringMethod.allCases.filter { $0 != .unknown },
            current: plant.wateringMethod
        )
    }

    private var accessibilityOptions: [PlantAccessibility] {
        withCurrent(
            PlantAccessibility.allCases.filter { $0 != .unknown },
            current: plant.accessibility
        )
    }

    private var drainageOptions: [PlantEditForm.DrainageChoice] {
        plant.hasDrainage == nil ? [.unrecorded, .drains, .sealed] : [.drains, .sealed]
    }

    private func drainageLabel(_ choice: PlantEditForm.DrainageChoice) -> String {
        switch choice {
        case .unrecorded: "Unrecorded"
        case .drains: "Has holes"
        case .sealed: "No holes"
        }
    }

    private func withCurrent<T: Hashable>(_ options: [T], current: T) -> [T] {
        options.contains(current) ? options : options + [current]
    }
}
