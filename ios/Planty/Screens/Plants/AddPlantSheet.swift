import SwiftUI

/// Photo-first in spirit: a name is the only required field, and everything
/// else can be corrected later. Open household vocabularies come from the
/// server and always retain an explicit custom-value escape hatch.
struct AddPlantSheet: View {
    let choices: ManagedChoicesStore
    let create: (NewPlant) async -> PlantyError?

    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var place = ""
    @State private var owner = ""
    @State private var potMaterial = ""
    @State private var domain = PlantDomain.houseplant
    @State private var watering = WateringMethod.hand
    @State private var action = AsyncSheetAction()

    var body: some View {
        NavigationStack {
            Form {
                if let error = action.error {
                    Section {
                        SheetErrorRow(
                            headline: "Not saved. Everything you typed is still here.",
                            error: error
                        )
                    }
                }
                Section("What is it") {
                    TextField("Common name", text: $name)
                    ManagedChoiceField(
                        title: "Place",
                        emptyLabel: "Choose a place",
                        customLabel: "Add a custom place",
                        choices: choices.catalog.places,
                        value: $place
                    )
                }
                Section("Whose is it") {
                    ManagedChoiceField(
                        title: "Owner",
                        emptyLabel: "Yours",
                        customLabel: "Add a custom owner",
                        choices: choices.catalog.owners,
                        value: $owner
                    )
                }
                Section("How it grows") {
                    Picker("Kind", selection: $domain) {
                        ForEach(PlantDomain.allCases.filter { $0 != .unknown }, id: \.self) {
                            Text($0.label).tag($0)
                        }
                    }
                    Picker("Watering", selection: $watering) {
                        ForEach(WateringMethod.allCases.filter { $0 != .unknown }, id: \.self) {
                            Text($0.label).tag($0)
                        }
                    }
                }
                Section("The pot") {
                    ManagedChoiceField(
                        title: "Material",
                        emptyLabel: "Not recorded",
                        customLabel: "Add a custom material",
                        choices: choices.catalog.potMaterials,
                        value: $potMaterial
                    )
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Add a plant")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                        .disabled(action.isRunning)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button {
                        Task { await submit() }
                    } label: {
                        if action.isRunning {
                            ProgressView()
                        } else {
                            Text("Add")
                        }
                    }
                    .disabled(action.isRunning || name.trimmingCharacters(in: .whitespaces).isEmpty)
                    .accessibilityLabel(action.isRunning ? "Saving" : "Add the plant")
                }
            }
            .task { await choices.loadIfNeeded() }
        }
        .interactiveDismissDisabled(action.isRunning)
    }

    /// The sheet closes only once the plant exists. A failure keeps every
    /// field as typed, because retyping a botanical name is how plants end up
    /// never recorded at all.
    private func submit() async {
        guard await action.perform({ await create(draft) }) else { return }
        await choices.load()
        dismiss()
    }

    private var draft: NewPlant {
        var draft = NewPlant(commonName: name.trimmingCharacters(in: .whitespaces))
        let selectedPlace = place.cleaned
        if !selectedPlace.isEmpty {
            // One place drives both legacy storage fields so new records cannot
            // split a room from the Home Assistant area that represents it.
            draft.location = selectedPlace
            draft.haArea = selectedPlace
        }
        let trimmedOwner = owner.cleaned
        draft.steward = trimmedOwner.isEmpty ? nil : trimmedOwner
        let trimmedMaterial = potMaterial.cleaned
        draft.potMaterial = trimmedMaterial.isEmpty ? nil : trimmedMaterial
        draft.domain = domain
        draft.wateringMethod = watering
        return draft
    }
}

/// One row that says a write failed without closing anything or losing input.
/// Lives in a Form section so every entry sheet fails the same way.
struct SheetErrorRow: View {
    let headline: String
    let error: PlantyError

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Label(headline, systemImage: "exclamationmark.triangle.fill")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(PlantyColor.orange)
            if let detail = error.errorDescription {
                Text(detail)
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .listRowBackground(PlantyColor.orange.opacity(0.12))
        .accessibilityElement(children: .combine)
    }
}
