import SwiftUI
import UIKit

/// The destination for a Today card. Looking, postponing, recording care and
/// taking a photo are separate choices so none of them is accidentally implied
/// by tapping the card itself.
struct TodayActionScreen: View {
    let entry: DigestEntry

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss

    @State private var isHandling = false
    @State private var isAddingPhoto = false
    @State private var isSavingPhoto = false
    @State private var photoPreview: Data?
    @State private var photoSaved = false

    private var store: TodayStore { session.today }
    private var state: CareState { CareState.from(action: entry.verdict.action) }

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                PlantPhotoView(plant: entry.plant, height: 240)
                identity
                recommendation
                photoSection
                actionSection
                reminderSection
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 20)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .plantyPage()
        .navigationTitle(entry.plant.commonName)
        .navigationBarTitleDisplayMode(.inline)
        .sheet(isPresented: $isHandling) {
            HandledSheet(entry: entry) { kind, note in
                Task { await record(kind, note: note) }
            } noteOnly: { text in
                Task { await saveNote(text) }
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

            if !entry.verdict.evidence.isEmpty {
                DisclosureGroup("Why Planty thinks this") {
                    EvidenceDetail(verdict: entry.verdict)
                        .padding(.top, 8)
                }
                .font(.footnote.weight(.semibold))
                .tint(PlantyColor.cyan)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: state.color.opacity(0.4))
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

    private var actionSection: some View {
        VStack(alignment: .leading, spacing: 12) {
            SectionHeading(text: "Done with this?")

            Button("I already handled it") {
                if needsRecordedAction {
                    isHandling = true
                } else {
                    Task { await acknowledgeAndDismiss() }
                }
            }
            .buttonStyle(PrimaryButtonStyle(color: state.color))

            if needsRecordedAction {
                Text("Record what you did so the plant's story stays useful.")
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)

                Button("Acknowledge without logging care") {
                    Task { await acknowledgeAndDismiss() }
                }
                .buttonStyle(SecondaryButtonStyle())

                Text("This clears today's card without claiming that watering, harvesting, moving, or another care action happened.")
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            } else {
                Text("For a Watch item, this simply records that you looked and clears the current reminder.")
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
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

    private var needsRecordedAction: Bool {
        switch entry.verdict.action {
        case .water, .harvest, .urgent:
            true
        case .check, .none, .unknown:
            false
        }
    }

    private func acknowledgeAndDismiss() async {
        guard await store.acknowledge(entry) == nil else { return }
        dismiss()
    }

    private func record(_ kind: ObservationKind, note: String) async {
        isHandling = false
        guard await store.complete(entry, kind: kind, note: note) == nil else { return }
        dismiss()
    }

    private func saveNote(_ text: String) async {
        await store.addNote(entry, text: text)
        isHandling = false
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
