import SwiftUI

/// Photo-first in spirit: a name is the only required field, and everything
/// else can be corrected later. No numeric care entry anywhere.
struct AddPlantSheet: View {
    let create: (NewPlant) async -> PlantyError?

    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var location = ""
    @State private var owner = ""
    @State private var domain = PlantDomain.houseplant
    @State private var watering = WateringMethod.hand
    @State private var error: PlantyError?
    @State private var isSaving = false

    var body: some View {
        NavigationStack {
            Form {
                if let error {
                    Section {
                        SheetErrorRow(
                            headline: "Not saved. Everything you typed is still here.",
                            error: error
                        )
                    }
                }
                Section("What is it") {
                    TextField("Common name", text: $name)
                    TextField("Where it lives", text: $location)
                }
                Section("Whose is it") {
                    TextField("Owner's name, or leave blank for yours", text: $owner)
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
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Add a plant")
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
                            Text("Add")
                        }
                    }
                    .disabled(isSaving || name.trimmingCharacters(in: .whitespaces).isEmpty)
                    .accessibilityLabel(isSaving ? "Saving" : "Add the plant")
                }
            }
        }
        .interactiveDismissDisabled(isSaving)
    }

    /// The sheet closes only once the plant exists. A failure keeps every
    /// field as typed, because retyping a botanical name is how plants end up
    /// never recorded at all.
    private func submit() async {
        isSaving = true
        defer { isSaving = false }
        error = await create(draft)
        if error == nil { dismiss() }
    }

    private var draft: NewPlant {
        var draft = NewPlant(commonName: name.trimmingCharacters(in: .whitespaces))
        let trimmedLocation = location.trimmingCharacters(in: .whitespaces)
        draft.location = trimmedLocation.isEmpty ? nil : trimmedLocation
        let trimmedOwner = owner.trimmingCharacters(in: .whitespaces)
        draft.steward = trimmedOwner.isEmpty ? nil : trimmedOwner
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
