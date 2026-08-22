import SwiftUI

/// A closed picker over the models the server says may do one job. Not
/// ManagedChoiceField, which is for open vocabularies and offers a custom
/// value: here that would mean naming a model that cannot do the job.
struct ModelPickerField: View {
    let job: AIJob
    let choices: [AIModel]
    let current: AIModel?
    let isDefault: Bool
    let onChoose: (AIModel) async -> PlantyError?
    let onUseDefault: () async -> PlantyError?

    @State private var showingPicker = false

    var body: some View {
        Button {
            showingPicker = true
        } label: {
            HStack {
                Text(job.label)
                    .foregroundStyle(PlantyColor.foreground)
                Spacer()
                Text(current?.name ?? "Default")
                    .foregroundStyle(isDefault ? PlantyColor.secondaryText : PlantyColor.foreground)
                    .multilineTextAlignment(.trailing)
                Image(systemName: "chevron.right")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .buttonStyle(.plain)
        .sheet(isPresented: $showingPicker) {
            ModelPicker(
                job: job,
                choices: choices,
                current: current,
                isDefault: isDefault,
                onChoose: onChoose,
                onUseDefault: onUseDefault
            )
        }
    }
}

private struct ModelPicker: View {
    let job: AIJob
    let choices: [AIModel]
    let current: AIModel?
    let isDefault: Bool
    let onChoose: (AIModel) async -> PlantyError?
    let onUseDefault: () async -> PlantyError?

    @Environment(\.dismiss) private var dismiss
    @State private var action = AsyncSheetAction()
    @State private var failure: String?

    var body: some View {
        NavigationStack {
            List {
                Section {
                    row(name: "Default", detail: job.defaultDetail, selected: isDefault) {
                        await onUseDefault()
                    }
                } footer: {
                    Text(job.explanation)
                }

                Section("Models that can do this") {
                    ForEach(choices) { model in
                        row(name: model.name, detail: model.note, selected: model.ref == current?.ref) {
                            await onChoose(model)
                        }
                    }
                }

                if let failure {
                    Section {
                        Text(failure).foregroundStyle(PlantyColor.red)
                    }
                }
            }
            .listStyle(.insetGrouped)
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle(job.label)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Done") { dismiss() }
                }
            }
        }
    }

    @ViewBuilder
    private func row(name: String, detail: String?, selected: Bool,
                     choose: @escaping () async -> PlantyError?) -> some View {
        Button {
            Task {
                let saved = await action.perform { await choose() }
                if saved {
                    failure = nil
                    dismiss()
                } else {
                    failure = action.error?.errorDescription
                }
            }
        } label: {
            HStack(alignment: .firstTextBaseline) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(name).foregroundStyle(PlantyColor.foreground)
                    if let detail, !detail.isEmpty {
                        Text(detail)
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                }
                Spacer()
                if selected {
                    Image(systemName: "checkmark")
                        .foregroundStyle(PlantyColor.green)
                }
            }
        }
        .buttonStyle(.plain)
        .disabled(action.isRunning)
        .listRowBackground(PlantyColor.surface)
        .accessibilityValue(selected ? "Selected" : "")
    }
}
