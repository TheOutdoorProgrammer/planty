import SwiftUI

/// A harvest is a number, not a diary line: yield per season has to add up,
/// which is why this is not just another observation with a body.
struct HarvestSheet: View {
    let plantName: String
    let log: (Double, String, String?) async -> PlantyError?

    @Environment(\.dismiss) private var dismiss
    @State private var quantityText = ""
    @State private var unit = "oz"
    @State private var notes = ""
    @State private var error: PlantyError?
    @State private var validation: String?
    @State private var isSaving = false

    private static let units = ["oz", "lb", "g", "kg", "items"]

    var body: some View {
        NavigationStack {
            Form {
                if let error {
                    Section {
                        SheetErrorRow(
                            headline: "Not recorded. The numbers are still here.",
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
                Section("How much") {
                    LabeledContent("Amount") {
                        TextField("0", text: $quantityText)
                            .keyboardType(.decimalPad)
                            .multilineTextAlignment(.trailing)
                            .accessibilityLabel("Amount harvested")
                    }
                    Picker("Unit", selection: $unit) {
                        ForEach(Self.units, id: \.self) { Text($0).tag($0) }
                    }
                    .pickerStyle(.segmented)
                    .accessibilityLabel("Unit")
                }
                Section {
                    TextField("Anything worth noting? (optional)", text: $notes, axis: .vertical)
                        .lineLimit(2...5)
                        .accessibilityLabel("Harvest notes")
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Harvest from \(plantName)")
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
                    .disabled(isSaving || quantityText.cleaned.isEmpty)
                    .accessibilityLabel(isSaving ? "Saving" : "Save the harvest")
                }
            }
        }
        .interactiveDismissDisabled(isSaving)
    }

    private func submit() async {
        validation = nil
        let cleanedQuantity = quantityText.cleaned.replacingOccurrences(of: ",", with: ".")
        guard let quantity = Double(cleanedQuantity), quantity > 0 else {
            validation = "The amount has to be a number above zero, like 3 or 0.5."
            return
        }
        isSaving = true
        defer { isSaving = false }
        let trimmedNotes = notes.cleaned
        error = await log(quantity, unit, trimmedNotes.isEmpty ? nil : trimmedNotes)
        if error == nil { dismiss() }
    }
}
