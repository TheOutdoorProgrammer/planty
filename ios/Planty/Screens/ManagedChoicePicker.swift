import SwiftUI

/// One reusable control for user-owned vocabularies. Closed protocol concepts
/// remain Pickers backed by enums; these values stay editable household data.
struct ManagedChoiceField: View {
    let title: String
    let emptyLabel: String
    let customLabel: String
    let choices: ManagedChoiceList
    @Binding var value: String

    @State private var showingPicker = false

    var body: some View {
        Button {
            showingPicker = true
        } label: {
            HStack {
                Text(title)
                    .foregroundStyle(PlantyColor.foreground)
                Spacer()
                Text(value.cleaned.isEmpty ? emptyLabel : value.cleaned)
                    .foregroundStyle(value.cleaned.isEmpty ? PlantyColor.secondaryText : PlantyColor.foreground)
                    .multilineTextAlignment(.trailing)
                Image(systemName: "chevron.right")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .buttonStyle(.plain)
        .sheet(isPresented: $showingPicker) {
            ManagedChoicePicker(
                title: title,
                customLabel: customLabel,
                choices: choices,
                selection: $value
            )
        }
    }
}

private struct ManagedChoicePicker: View {
    let title: String
    let customLabel: String
    let choices: ManagedChoiceList
    @Binding var selection: String

    @Environment(\.dismiss) private var dismiss
    @State private var search = ""
    @State private var customValue = ""
    @State private var enteringCustom = false

    var body: some View {
        NavigationStack {
            List {
                if !recent.isEmpty {
                    Section("Recently used") {
                        ForEach(recent) { choice in row(choice) }
                    }
                }

                if !other.isEmpty {
                    Section(recent.isEmpty ? "Choices" : "All choices") {
                        ForEach(other) { choice in row(choice) }
                    }
                }

                Section {
                    if enteringCustom {
                        TextField("Custom value", text: $customValue)
                            .textInputAutocapitalization(.words)
                        Button("Use custom value") {
                            let value = customValue.cleaned
                            guard !value.isEmpty else { return }
                            selection = value
                            dismiss()
                        }
                        .disabled(customValue.cleaned.isEmpty)
                    } else {
                        Button(customLabel) {
                            customValue = selection
                            enteringCustom = true
                        }
                    }
                }

                if recent.isEmpty && other.isEmpty && !enteringCustom {
                    Section {
                        ContentUnavailableView(
                            search.cleaned.isEmpty ? "No saved choices yet" : "No matching choices",
                            systemImage: "text.badge.plus",
                            description: Text("Use the custom option to add the value you need.")
                        )
                    }
                }
            }
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .searchable(text: $search, prompt: "Search")
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
            }
        }
    }

    private var matching: [ManagedChoice] {
        let needle = search.cleaned.lowercased()
        guard !needle.isEmpty else { return choices.all }
        return choices.all.filter { $0.value.lowercased().contains(needle) }
    }

    private var recent: [ManagedChoice] {
        let allowed = Set(matching.map(\.id))
        return choices.recent.filter { allowed.contains($0.id) }
    }

    private var other: [ManagedChoice] {
        let recentIDs = Set(recent.map(\.id))
        return matching.filter { !recentIDs.contains($0.id) }
    }

    private func row(_ choice: ManagedChoice) -> some View {
        Button {
            selection = choice.value
            dismiss()
        } label: {
            HStack {
                VStack(alignment: .leading, spacing: 3) {
                    Text(choice.value)
                        .foregroundStyle(PlantyColor.foreground)
                    if choice.sources.contains("home_assistant") {
                        Text("Home Assistant area")
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                }
                Spacer()
                if choice.value == selection {
                    Image(systemName: "checkmark")
                        .font(.body.weight(.semibold))
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .contentShape(Rectangle())
        }
        .accessibilityLabel(choice.value)
        .accessibilityValue(choice.value == selection ? "Selected" : "")
    }
}
