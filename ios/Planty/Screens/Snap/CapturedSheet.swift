import SwiftUI

/// After capture, the screen asks for the outcome in one scan-friendly grid.
/// Every pre-existing action remains available; the hierarchy simply separates
/// "record what happened" from "ask about this picture" and "save only".
struct CapturedSheet: View {
    let photo: CapturedPhoto
    let plant: Plant?
    @Binding var note: String
    let isBusy: Bool
    let record: (ObservationKind?) -> Void
    let lookOff: () -> Void
    let retake: () -> Void
    let justAsk: () -> Void
    var identification: IdentificationStore?
    var useCandidate: ((IdentificationCandidate) -> Void)?

    private let columns = [GridItem(.flexible()), GridItem(.flexible())]

    var body: some View {
        VStack(alignment: .leading, spacing: 18) {
            preview

            if plant == nil {
                UnknownPlantCard()
            }

            if let identification {
                IdentificationView(store: identification) { candidate in
                    useCandidate?(candidate)
                }
            }

            VStack(alignment: .leading, spacing: 10) {
                SectionHeading("What happened?", detail: "Choose one if this photo records care.")

                LazyVGrid(columns: columns, spacing: 10) {
                    CaptureAction(title: "Watered", icon: "drop.fill", color: PlantyColor.cyan) {
                        record(.watered)
                    }
                    CaptureAction(title: "Misted", icon: "cloud.drizzle.fill", color: PlantyColor.cyan) {
                        record(.misted)
                    }
                    CaptureAction(title: "Repotted", icon: "arrow.triangle.2.circlepath", color: PlantyColor.green) {
                        record(.repotted)
                    }
                    CaptureAction(title: "Looks off", icon: "exclamationmark.magnifyingglass", color: PlantyColor.orange) {
                        lookOff()
                    }
                }
            }

            noteField

            VStack(alignment: .leading, spacing: 10) {
                SectionHeading("Need an answer?", detail: "Ask about this exact photo without creating anything new.")
                Button(action: justAsk) {
                    Label("Ask Planty about this photo", systemImage: "bubble.left.and.text.bubble.right.fill")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(SecondaryButtonStyle())
            }

            Button("Save photo only") { record(nil) }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.green))

            Button("Retake", action: retake)
                .font(.subheadline.weight(.semibold))
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
                PlantyColor.surface
            }
        }
        .frame(maxWidth: .infinity)
        .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
        .accessibilityLabel(
            plant.map { "Photo just taken of \($0.commonName)" } ?? "Photo just taken"
        )
    }

    private var noteField: some View {
        VStack(alignment: .leading, spacing: 6) {
            Text("Note (optional)")
                .font(.footnote.weight(.semibold))
                .foregroundStyle(PlantyColor.secondaryText)
            TextField("Anything worth remembering?", text: $note, axis: .vertical)
                .textFieldStyle(.plain)
                .lineLimit(1...4)
                .padding(12)
                .background(
                    PlantyColor.surface,
                    in: RoundedRectangle(cornerRadius: 14, style: .continuous)
                )
                .overlay {
                    RoundedRectangle(cornerRadius: 14, style: .continuous)
                        .stroke(PlantyColor.quietDecoration.opacity(0.18), lineWidth: 1)
                }
                .accessibilityLabel("Optional note about this photo")
        }
    }

    private var savingOverlay: some View {
        ProgressView("Saving photo…")
            .tint(PlantyColor.pink)
            .padding(20)
            .background(.ultraThinMaterial, in: RoundedRectangle(cornerRadius: 18))
    }
}

private struct CaptureAction: View {
    let title: String
    let icon: String
    let color: Color
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            VStack(alignment: .leading, spacing: 10) {
                Image(systemName: icon)
                    .font(.headline)
                    .foregroundStyle(color)
                    .frame(width: 34, height: 34)
                    .background(color.opacity(0.1), in: Circle())
                Text(title)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(PlantyColor.foreground)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            .padding(12)
            .frame(maxWidth: .infinity, minHeight: 92, alignment: .leading)
            .background(PlantyColor.surface, in: RoundedRectangle(cornerRadius: 16, style: .continuous))
            .overlay {
                RoundedRectangle(cornerRadius: 16, style: .continuous)
                    .stroke(color.opacity(0.14), lineWidth: 1)
            }
        }
        .buttonStyle(.plain)
    }
}

struct UnknownPlantCard: View {
    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            Label("Choose the plant before saving", systemImage: "questionmark.circle.fill")
                .font(.headline)
                .foregroundStyle(PlantyColor.orange)
            Text("Use the picker above so the photo lands in the right story.")
                .font(.footnote)
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: PlantyColor.orange.opacity(0.2), padding: 14)
    }
}

struct PlantPickerSheet: View {
    let plants: [Plant]
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
                        Text("Leave the name empty and Planty will work it out from the picture.")
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                }
                ForEach(groups) { group in
                    Section(group.title) {
                        ForEach(group.plants) { plant in
                            Button { pick(plant) } label: {
                                PlantRowLabel(plant: plant)
                            }
                        }
                    }
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .searchable(text: $search, prompt: "Name, room, species or owner")
            .navigationTitle("Choose a plant")
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
