import SwiftUI

/// Three large exception-oriented actions. No moisture field, no checklist, no
/// required note: routine observation belongs to the sensors.
struct CapturedSheet: View {
    let photo: CapturedPhoto
    let plant: Plant?
    @Binding var note: String
    let isBusy: Bool
    let record: (ObservationKind?) -> Void
    let lookOff: () -> Void
    let retake: () -> Void

    /// Identification sits with the unknown-plant prompt because that is the
    /// moment somebody is deciding what this actually is.
    var identification: IdentificationStore?
    var useCandidate: ((IdentificationCandidate) -> Void)?

    var body: some View {
        VStack(alignment: .leading, spacing: 16) {
            preview

            if plant == nil {
                UnknownPlantCard()
            }

            if let identification {
                IdentificationView(store: identification) { candidate in
                    useCandidate?(candidate)
                }
            }

            Text("What happened here?")
                .font(.title3.weight(.bold))

            Button("Watered by hand") { record(.watered) }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.cyan))
            Button("Repotted") { record(.repotted) }
                .buttonStyle(SecondaryButtonStyle())
            Button("Something looks off", action: lookOff)
                .buttonStyle(SecondaryButtonStyle())

            noteField

            Button("Save photo only") { record(nil) }
                .buttonStyle(SecondaryButtonStyle())
            Button("Retake", action: retake)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
                .frame(maxWidth: .infinity, minHeight: 44)
        }
        .disabled(isBusy)
        .overlay { if isBusy { savingOverlay } }
    }

    private var preview: some View {
        Group {
            if let image = UIImage(data: photo.jpeg) {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFit()
            } else {
                Color(hex: 0x1F2D2A)
            }
        }
        .frame(maxWidth: .infinity)
        .clipShape(RoundedRectangle(cornerRadius: 24, style: .continuous))
        .accessibilityLabel(
            plant.map { "Photo just taken of \($0.commonName)" } ?? "Photo just taken"
        )
    }

    /// Never focused automatically: the whole point is that it is optional.
    private var noteField: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Add a short note (optional)")
                .font(.footnote)
                .foregroundStyle(PlantyColor.secondaryText)
            TextField("", text: $note, axis: .vertical)
                .textFieldStyle(.plain)
                .lineLimit(1...4)
                .padding(12)
                .background(
                    PlantyColor.background,
                    in: RoundedRectangle(cornerRadius: 14, style: .continuous)
                )
                .overlay {
                    RoundedRectangle(cornerRadius: 14, style: .continuous)
                        .stroke(PlantyColor.quietDecoration.opacity(0.5), lineWidth: 1)
                }
                .accessibilityLabel("Optional note about this photo")
        }
    }

    private var savingOverlay: some View {
        ProgressView("Saving the photo first…")
            .tint(PlantyColor.pink)
            .padding(20)
            .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: 20))
    }
}

/// A low-confidence guess is never accepted silently.
struct UnknownPlantCard: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Which plant is this?")
                .font(.headline)
            Text("Pick one above before saving, so this lands in the right story.")
                .font(.footnote)
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: PlantyColor.yellow.opacity(0.5), padding: 14)
    }
}

/// Recent first, then friend groups, then mine, matching the library order.
struct PlantPickerSheet: View {
    let plants: [Plant]
    /// Offered alongside the list, and the only option at all on a first run.
    /// Without it an empty library was a dead end with the photo discarded.
    var addNew: ((String?) -> Void)?
    let pick: (Plant) -> Void

    @State private var search = ""
    @State private var newName = ""

    var body: some View {
        NavigationStack {
            List {
                if addNew != nil {
                    Section(plants.isEmpty ? "Your first plant" : "Not on the list?") {
                        TextField("What is it called?", text: $newName)
                        Button {
                            let trimmed = newName.trimmingCharacters(in: .whitespacesAndNewlines)
                            addNew?(trimmed.isEmpty ? nil : trimmed)
                        } label: {
                            Label(
                                newName.trimmingCharacters(in: .whitespaces).isEmpty
                                    ? "Add it from this photo"
                                    : "Add \(newName) from this photo",
                                systemImage: "plus.circle.fill"
                            )
                            .frame(maxWidth: .infinity, alignment: .leading)
                        }
                        .listRowBackground(PlantyColor.surface)
                        Text("Leave the name empty and Planty will work it out from the picture.")
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                            .listRowBackground(PlantyColor.surface)
                    }
                }
                ForEach(groups) { group in
                    Section(group.title) {
                        ForEach(group.plants) { plant in
                            Button { pick(plant) } label: {
                                PlantRowLabel(plant: plant)
                            }
                            .listRowBackground(PlantyColor.surface)
                        }
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .searchable(text: $search, prompt: "Name, room, species or owner")
            .navigationTitle("Pick a plant")
            .navigationBarTitleDisplayMode(.inline)
        }
    }

    private var groups: [PlantGroup] {
        let needle = search.trimmingCharacters(in: .whitespacesAndNewlines)
        let filtered = needle.isEmpty ? plants : plants.filter { plant in
            [plant.commonName, plant.location, plant.steward, plant.botanicalName ?? ""]
                .contains { $0.localizedCaseInsensitiveContains(needle) }
        }
        let friends = Dictionary(grouping: filtered.filter(\.isFriends), by: \.steward)
        var groups = friends.keys.sorted().map {
            PlantGroup(title: "\($0)'s plants", isFriendOwned: true, plants: friends[$0] ?? [])
        }
        let mine = filtered.filter { !$0.isFriends }
        if !mine.isEmpty {
            groups.append(PlantGroup(title: "Mine", isFriendOwned: false, plants: mine))
        }
        return groups
    }
}

struct PlantRowLabel: View {
    let plant: Plant

    var body: some View {
        HStack(spacing: 12) {
            VStack(alignment: .leading, spacing: 2) {
                Text(plant.commonName)
                    .font(.headline)
                    .foregroundStyle(PlantyColor.foreground)
                Text(plant.location)
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            Spacer(minLength: 8)
            OwnershipBadge(plant: plant)
        }
        .frame(minHeight: 44)
    }
}
