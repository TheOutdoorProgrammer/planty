import SwiftUI

/// Photo-first in spirit: a name is the only required field, and everything
/// else can be corrected later. No numeric care entry anywhere.
struct AddPlantSheet: View {
    let create: (NewPlant) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var name = ""
    @State private var location = ""
    @State private var owner = ""
    @State private var domain = PlantDomain.houseplant
    @State private var watering = WateringMethod.hand

    var body: some View {
        NavigationStack {
            Form {
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
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Add") { create(draft) }
                        .disabled(name.trimmingCharacters(in: .whitespaces).isEmpty)
                }
            }
        }
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
