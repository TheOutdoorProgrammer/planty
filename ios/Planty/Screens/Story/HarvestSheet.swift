import SwiftUI

/// A harvest is a number, not a diary line: yield per season has to add up,
/// which is why this is not just another observation with a body.
struct HarvestSheet: View {
    let plantName: String
    let harvest: Harvest?
    let log: (Double, String, String?) async -> PlantyError?

    @Environment(\.dismiss) private var dismiss
    @State private var quantityText = ""
    @State private var unit = "oz"
    @State private var notes = ""
    @State private var validation: String?
    @State private var action = AsyncSheetAction()

    private static let units = ["oz", "lb", "g", "kg", "items"]

    init(
        plantName: String,
        harvest: Harvest? = nil,
        log: @escaping (Double, String, String?) async -> PlantyError?
    ) {
        self.plantName = plantName
        self.harvest = harvest
        self.log = log
        _quantityText = State(initialValue: harvest?.quantity.formatted() ?? "")
        _unit = State(initialValue: harvest?.unit ?? "oz")
        _notes = State(initialValue: harvest?.notes ?? "")
    }

    var body: some View {
        NavigationStack {
            Form {
                if let error = action.error {
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
            .navigationTitle(harvest == nil ? "Harvest from \(plantName)" : "Edit harvest")
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
                            Text("Save")
                        }
                    }
                    .disabled(action.isRunning || quantityText.cleaned.isEmpty)
                    .accessibilityLabel(action.isRunning ? "Saving" : "Save the harvest")
                }
            }
        }
        .interactiveDismissDisabled(action.isRunning)
    }

    private func submit() async {
        validation = nil
        let cleanedQuantity = quantityText.cleaned.replacingOccurrences(of: ",", with: ".")
        guard let quantity = Double(cleanedQuantity), quantity > 0 else {
            validation = "The amount has to be a number above zero, like 3 or 0.5."
            return
        }
        let trimmedNotes = notes.cleaned
        guard await action.perform({
            await log(quantity, unit, trimmedNotes.isEmpty ? nil : trimmedNotes)
        }) else { return }
        dismiss()
    }
}
