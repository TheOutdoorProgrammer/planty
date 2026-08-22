import SwiftUI
import UIKit

/// The destination for a Today card. The things a person may actually have done
/// are first-class actions here; clearing without a record remains available,
/// but it is no longer the only obvious way to finish a card.
struct TodayActionScreen: View {
    let entry: DigestEntry

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss

    @State private var isAddingPhoto = false
    @State private var isSavingPhoto = false
    @State private var photoPreview: Data?
    @State private var photoSaved = false
    @State private var isWritingDetail = false
    @State private var writingKind: ObservationKind = .note

    private let actionColumns = [GridItem(.flexible()), GridItem(.flexible())]
    private let resolutionKinds: [ObservationKind] = [
        .watered, .misted, .fertilized, .repotted,
        .pruned, .moved, .harvested, .symptom, .note
    ]

    private var store: TodayStore { session.today }
    private var state: CareState { CareState.from(action: entry.verdict.action) }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                PlantPhotoView(plant: entry.plant, height: 240)
                identity
                recommendation
                actionSection
                photoSection
                reminderSection
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 20)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .plantyPage()
        .navigationTitle(entry.plant.commonName)
        .navigationBarTitleDisplayMode(.inline)
        .sheet(isPresented: $isWritingDetail) {
            ResolutionDetailSheet(
                plantName: entry.plant.commonName,
                kind: writingKind
            ) { text in
                Task { await record(writingKind, note: text) }
            }
        }
        .sheet(isPresented: $isAddingPhoto) {
            PhotoAttachSheet(
                title: "Photo of \(entry.plant.commonName)",
                guidance: "Frame \(entry.plant.commonName) clearly.",
                footnote: "Optional. This photo is added to the plant's story; it does not clear the Today card by itself."
            ) { jpeg in
                Task { await savePhoto(jpeg) }
            }
        }
    }

    private var identity: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack(spacing: 10) {
                OwnershipBadge(plant: entry.plant)
                StatusPill(state: state)
            }
            Text(entry.plant.commonName)
                .font(.largeTitle.weight(.bold))
            if !subtitle.isEmpty {
                Text(subtitle)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
    }

    private var subtitle: String {
        [entry.plant.displaySpecies, entry.plant.location]
            .compactMap { $0 }
            .filter { !$0.isEmpty }
            .joined(separator: " · ")
    }

    private var recommendation: some View {
        VStack(alignment: .leading, spacing: 10) {
            Eyebrow(text: entry.verdict.action.shortLabel, color: state.color)
            Text(entry.verdict.action.instruction)
                .font(.title3.weight(.bold))

            if !entry.verdict.reasoning.isEmpty {
                Text(entry.verdict.reasoning)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            recommendationTools
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: state.color.opacity(0.4))
    }

    /// The explanation and its follow-up conversation are peers. On a narrow
    /// screen or at a large Dynamic Type size they stack instead of competing
    /// for a cramped row.
    @ViewBuilder
    private var recommendationTools: some View {
        if entry.verdict.evidence.isEmpty {
            chatLink
        } else {
            ViewThatFits(in: .horizontal) {
                HStack(alignment: .top, spacing: 12) {
                    evidenceDisclosure
                    Spacer(minLength: 8)
                    chatLink
                }
                VStack(alignment: .leading, spacing: 6) {
                    evidenceDisclosure
                    chatLink
                }
            }
        }
    }

    private var evidenceDisclosure: some View {
        DisclosureGroup("Why Planty thinks this") {
            EvidenceDetail(verdict: entry.verdict)
                .padding(.top, 8)
        }
        .font(.footnote.weight(.semibold))
        .tint(PlantyColor.cyan)
    }

    private var chatLink: some View {
        NavigationLink {
            ConsultScreen(store: session.consultStore(for: entry))
        } label: {
            Label(
                "Chat with Planty",
                systemImage: "bubble.left.and.text.bubble.right.fill"
            )
        }
        .buttonStyle(.plain)
        .font(.footnote.weight(.semibold))
        .foregroundStyle(PlantyColor.pink)
        .frame(minHeight: 44)
        .accessibilityLabel(
            "Chat with Planty about this finding for \(entry.plant.commonName)"
        )
    }

    private var actionSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            SectionHeading(text: "What did you do?")
            Text("Pick what happened. Any of these records it in the plant's story and clears this attention item.")
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)

            LazyVGrid(columns: actionColumns, spacing: 10) {
                ForEach(resolutionKinds, id: \.self) { kind in
                    Button {
                        choose(kind)
                    } label: {
                        Label(kind.label, systemImage: kind.symbol)
                            .frame(maxWidth: .infinity, alignment: .leading)
                    }
                    .buttonStyle(SecondaryButtonStyle())
                    .accessibilityLabel("\(kind.label) \(entry.plant.commonName)")
                }
            }

            Divider().overlay(PlantyColor.quietDecoration)

            Button("Clear without logging anything") {
                Task { await acknowledgeAndDismiss() }
            }
            .buttonStyle(SecondaryButtonStyle())

            Text("Use this only when the card is no longer relevant and you do not want to add an event to the plant's history.")
                .font(.caption)
                .foregroundStyle(PlantyColor.secondaryText)
        }
    }

    private var photoSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            SectionHeading(text: "Photo")
            Text("A current photo can be useful, but it is never required to deal with this card.")
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)

            if let photoPreview, let image = UIImage(data: photoPreview) {
                Image(uiImage: image)
                    .resizable()
                    .scaledToFill()
                    .frame(maxWidth: .infinity)
                    .frame(height: 180)
                    .clipShape(RoundedRectangle(cornerRadius: 18, style: .continuous))
            }

            Button {
                isAddingPhoto = true
            } label: {
                if isSavingPhoto {
                    HStack(spacing: 8) {
                        ProgressView()
                        Text("Saving photo…")
                    }
                    .frame(maxWidth: .infinity)
                } else {
                    Label("Add a photo (optional)", systemImage: "camera.fill")
                        .frame(maxWidth: .infinity)
                }
            }
            .buttonStyle(SecondaryButtonStyle())
            .disabled(isSavingPhoto)

            if photoSaved {
                Label("Photo added to the story.", systemImage: "checkmark.circle.fill")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(PlantyColor.green)
            }
        }
    }

    private var reminderSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            SectionHeading(text: "Not yet")
            Text("Move this card out of the way without marking it handled.")
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)

            ForEach(PostponeInterval.allCases) { interval in
                Button(interval.label) {
                    store.postpone(entry, by: interval)
                    dismiss()
                }
                .buttonStyle(SecondaryButtonStyle())
            }
        }
    }

    private func choose(_ kind: ObservationKind) {
        if kind == .note || kind == .symptom {
            writingKind = kind
            isWritingDetail = true
        } else {
            Task { await record(kind, note: "") }
        }
    }

    private func acknowledgeAndDismiss() async {
        guard await store.acknowledge(entry) == nil else { return }
        dismiss()
    }

    private func record(_ kind: ObservationKind, note: String) async {
        isWritingDetail = false
        guard await store.complete(entry, kind: kind, note: note) == nil else { return }
        dismiss()
    }

    private func savePhoto(_ jpeg: Data) async {
        guard !isSavingPhoto else { return }
        isSavingPhoto = true
        photoSaved = false
        photoPreview = jpeg
        defer { isSavingPhoto = false }

        if await store.addPhoto(entry, jpeg: jpeg) == nil {
            photoSaved = true
        } else {
            photoPreview = nil
        }
    }
}

private struct ResolutionDetailSheet: View {
    let plantName: String
    let kind: ObservationKind
    let save: (String) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var detail = ""
    @FocusState private var focused: Bool

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 16) {
                Text(prompt)
                    .font(.title3.weight(.bold))
                Text("This will be saved to \(plantName)'s story and clear the current attention item.")
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)

                TextField("A sentence is plenty", text: $detail, axis: .vertical)
                    .lineLimit(3...6)
                    .focused($focused)
                    .padding(12)
                    .background(PlantyColor.surface, in: RoundedRectangle(cornerRadius: 14))

                Button(kind == .note ? "Save note and clear" : "Record and clear") {
                    save(detail.trimmingCharacters(in: .whitespacesAndNewlines))
                }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.green))
                .disabled(detail.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)

                Spacer(minLength: 0)
            }
            .padding(20)
            .plantyPage()
            .navigationTitle(kind.label)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
            }
        }
        .presentationDetents([.medium])
        .onAppear { focused = true }
    }

    private var prompt: String {
        kind == .note ? "What is worth remembering?" : "What did you notice?"
    }
}
