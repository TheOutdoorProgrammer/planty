import PhotosUI
import SwiftUI

/// One photograph acquired with the shared camera/library state. Callers supply
/// framing copy and decide what the bytes mean once a photo exists.
struct PhotoAttachSheet: View {
    let title: String
    let guidance: String
    let footnote: String
    let attach: (Data) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var acquisition = PhotoAcquisition()

    init(
        title: String = "Add a photo",
        guidance: String = "Frame whatever Planty asked to see.",
        footnote: String = "A close-up is fine. One photo per message.",
        attach: @escaping (Data) -> Void
    ) {
        self.title = title
        self.guidance = guidance
        self.footnote = footnote
        self.attach = attach
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 18) {
                    if let error = acquisition.error {
                        Label(error, systemImage: "exclamationmark.triangle.fill")
                            .font(.subheadline)
                            .foregroundStyle(PlantyColor.orange)
                            .frame(maxWidth: .infinity, alignment: .leading)
                            .padding(12)
                            .background(
                                PlantyColor.orange.opacity(0.12),
                                in: RoundedRectangle(cornerRadius: 14)
                            )
                    }
                    stage
                }
                .padding(.horizontal, 20)
                .padding(.vertical, 16)
            }
            .plantyPage()
            .navigationTitle(title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel") { dismiss() }
                }
            }
            .task { await acquisition.prepare() }
            .onDisappear { acquisition.stop() }
            .onChange(of: acquisition.photoItem) { _, item in
                guard item != nil else { return }
                Task { await importPhoto() }
            }
        }
    }

    @ViewBuilder
    private var stage: some View {
        switch acquisition.camera.availability {
        case .denied:
            CameraPermissionCard(photoItem: photoItemBinding)
        case .unavailable:
            NoCameraCard(photoItem: photoItemBinding)
        case .ready, .unknown:
            CameraStage(
                camera: acquisition.camera,
                photoItem: photoItemBinding,
                guidance: guidance,
                footnote: footnote,
                shutter: { Task { await shoot() } }
            )
        }
    }

    private var photoItemBinding: Binding<PhotosPickerItem?> {
        Binding(
            get: { acquisition.photoItem },
            set: { acquisition.photoItem = $0 }
        )
    }

    private func shoot() async {
        guard let photo = await acquisition.takePhoto() else { return }
        attach(photo.jpeg)
        dismiss()
    }

    private func importPhoto() async {
        guard let photo = await acquisition.importSelectedPhoto() else { return }
        attach(photo.jpeg)
        dismiss()
    }
}
