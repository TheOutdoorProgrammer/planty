import SwiftUI

struct ToxicityEditForm: Equatable {
    var cats: ToxicityRating
    var dogs: ToxicityRating
    var people: ToxicityRating
    var basis: ToxicityBasis
    var identifiedAs: String
    var principle: String
    var signs: String
    var parts: Set<String>
    var routes: Set<String>
    var notes: String
    var firstAid: String
    var source: String

    enum Issue: Error, Equatable {
        case missingBasis
        case missingPrinciple

        var message: String {
            switch self {
            case .missingBasis:
                "Say whether the ratings came from the source or were graded in Planty."
            case .missingPrinciple:
                "A moderate or severe rating needs the toxic principle that justifies it."
            }
        }
    }

    init(plant: Plant, toxicity: Toxicity?) {
        let toxicity = toxicity ?? Toxicity()
        cats = toxicity.cats
        dogs = toxicity.dogs
        people = toxicity.people
        basis = toxicity.basis
        identifiedAs = toxicity.identifiedAs ?? plant.botanicalName ?? ""
        principle = toxicity.principle ?? ""
        signs = toxicity.signs ?? ""
        parts = Set(toxicity.parts)
        routes = Set(toxicity.routes)
        notes = toxicity.notes ?? ""
        firstAid = toxicity.firstAid ?? ""
        source = toxicity.source ?? ""
    }

    func toxicity(checkedAt: Date = Date()) -> Result<Toxicity, Issue> {
        guard basis != .unknown else { return .failure(.missingBasis) }
        let urgent = [cats, dogs, people].contains { $0 == .moderate || $0 == .severe }
        guard !urgent || !principle.cleaned.isEmpty else { return .failure(.missingPrinciple) }

        return .success(
            Toxicity(
                cats: cats,
                dogs: dogs,
                people: people,
                basis: basis,
                identifiedAs: identifiedAs.nilIfBlank,
                principle: principle.nilIfBlank,
                signs: signs.nilIfBlank,
                parts: Self.partOptions.map(\.value).filter(parts.contains),
                routes: Self.routeOptions.map(\.value).filter(routes.contains),
                notes: notes.nilIfBlank,
                firstAid: firstAid.nilIfBlank,
                source: source.nilIfBlank,
                checkedAt: checkedAt
            )
        )
    }

    static let partOptions: [ToxicityOption] = [
        ToxicityOption(value: "all", label: "All parts"),
        ToxicityOption(value: "bulb", label: "Bulb"),
        ToxicityOption(value: "leaf", label: "Leaves"),
        ToxicityOption(value: "stem", label: "Stems"),
        ToxicityOption(value: "sap", label: "Sap"),
        ToxicityOption(value: "flower", label: "Flowers"),
        ToxicityOption(value: "fruit", label: "Fruit"),
        ToxicityOption(value: "seed", label: "Seeds"),
        ToxicityOption(value: "root", label: "Roots")
    ]

    static let routeOptions: [ToxicityOption] = [
        ToxicityOption(value: "eaten", label: "Eaten"),
        ToxicityOption(value: "skin", label: "Skin contact"),
        ToxicityOption(value: "eyes", label: "Eye contact"),
        ToxicityOption(value: "breathed", label: "Breathed in")
    ]
}

struct ToxicityOption: Identifiable, Hashable {
    let value: String
    let label: String
    var id: String { value }
}

struct ToxicityEditSheet: View {
    let plant: Plant
    let save: (Toxicity) async -> PlantyError?

    @Environment(\.dismiss) private var dismiss
    @State private var form: ToxicityEditForm
    @State private var validation: String?
    @State private var failure: PlantyError?
    @State private var saving = false

    init(
        plant: Plant,
        toxicity: Toxicity?,
        save: @escaping (Toxicity) async -> PlantyError?
    ) {
        self.plant = plant
        self.save = save
        _form = State(initialValue: ToxicityEditForm(plant: plant, toxicity: toxicity))
    }

    var body: some View {
        NavigationStack {
            Form {
                if let failure {
                    Section {
                        SheetErrorRow(
                            headline: "Not saved. Your toxicity notes are still here.",
                            error: failure
                        )
                    }
                }
                if let validation {
                    Section {
                        Label(validation, systemImage: "exclamationmark.triangle.fill")
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(PlantyColor.orange)
                    }
                    .listRowBackground(PlantyColor.orange.opacity(0.12))
                }

                Section {
                    ToxicityRatingPicker(audience: .cats, rating: $form.cats)
                    ToxicityRatingPicker(audience: .dogs, rating: $form.dogs)
                    ToxicityRatingPicker(audience: .people, rating: $form.people)
                } header: {
                    Text("Risk by audience")
                } footer: {
                    Text("Not checked is uncertainty, not a safer rating.")
                }

                Section {
                    Picker("How this was graded", selection: $form.basis) {
                        Text("Choose…").tag(ToxicityBasis.unknown)
                        Text("The source states it").tag(ToxicityBasis.source)
                        Text("Graded in Planty").tag(ToxicityBasis.derived)
                    }
                    TextField("Botanical name checked", text: $form.identifiedAs)
                        .textInputAutocapitalization(.never)
                    TextField("Source or URL", text: $form.source)
                        .textInputAutocapitalization(.never)
                        .keyboardType(.URL)
                        .autocorrectionDisabled()
                } header: {
                    Text("Evidence")
                } footer: {
                    Text("The species and source keep a confident-looking rating from becoming an untraceable guess.")
                }

                Section("What happens") {
                    TextField("Toxic principle", text: $form.principle, axis: .vertical)
                        .lineLimit(1...4)
                    TextField("Signs after exposure", text: $form.signs, axis: .vertical)
                        .lineLimit(2...6)
                    TextField("First aid when normal advice is wrong", text: $form.firstAid, axis: .vertical)
                        .lineLimit(2...6)
                }

                Section("Exposure") {
                    NavigationLink {
                        ToxicityOptionsScreen(
                            title: "Toxic parts",
                            options: ToxicityEditForm.partOptions,
                            selection: $form.parts
                        )
                    } label: {
                        ToxicitySelectionLabel(title: "Plant parts", count: form.parts.count)
                    }
                    NavigationLink {
                        ToxicityOptionsScreen(
                            title: "Exposure routes",
                            options: ToxicityEditForm.routeOptions,
                            selection: $form.routes
                        )
                    } label: {
                        ToxicitySelectionLabel(title: "Routes", count: form.routes.count)
                    }
                }

                Section("Context") {
                    TextField("Dose caveats or why risks differ", text: $form.notes, axis: .vertical)
                        .lineLimit(2...8)
                }
            }
            .plantyPage()
            .navigationTitle("Toxicity")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }.disabled(saving)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await submit() } }.disabled(saving)
                }
            }
        }
        .interactiveDismissDisabled(saving)
    }

    private func submit() async {
        validation = nil
        failure = nil
        guard case .success(let toxicity) = form.toxicity() else {
            if case .failure(let issue) = form.toxicity() { validation = issue.message }
            return
        }
        saving = true
        failure = await save(toxicity)
        saving = false
        if failure == nil { dismiss() }
    }
}

private struct ToxicityRatingPicker: View {
    let audience: ToxicityAudience
    @Binding var rating: ToxicityRating

    var body: some View {
        Picker(audience.label, selection: $rating) {
            ForEach(ToxicityRating.allCases, id: \.self) { option in
                Label(option.label, systemImage: option.symbol).tag(option)
            }
        }
        .tint(rating.color)
    }
}

private struct ToxicitySelectionLabel: View {
    let title: String
    let count: Int

    var body: some View {
        LabeledContent(title) {
            Text(count == 0 ? "None selected" : "\(count) selected")
                .foregroundStyle(PlantyColor.secondaryText)
        }
    }
}

private struct ToxicityOptionsScreen: View {
    let title: String
    let options: [ToxicityOption]
    @Binding var selection: Set<String>

    var body: some View {
        List(options) { option in
            Button {
                if selection.contains(option.value) {
                    selection.remove(option.value)
                } else {
                    selection.insert(option.value)
                }
            } label: {
                HStack {
                    Text(option.label)
                        .foregroundStyle(PlantyColor.foreground)
                    Spacer()
                    if selection.contains(option.value) {
                        Image(systemName: "checkmark")
                            .font(.headline)
                            .foregroundStyle(PlantyColor.green)
                    }
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .listRowBackground(PlantyColor.surface)
        }
        .plantyPage()
        .navigationTitle(title)
    }
}
