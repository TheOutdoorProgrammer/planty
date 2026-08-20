import PhotosUI
import SwiftUI

/// One photograph for the message being written. Reuses the shared acquisition
/// state, so camera permission, library import and failures behave like Snap.
struct PhotoAttachSheet: View {
    let attach: (Data) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var acquisition = PhotoAcquisition()

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
                            .background(PlantyColor.orange.opacity(0.12), in: RoundedRectangle(cornerRadius: 14))
                    }
                    stage
                }
                .padding(.horizontal, 20)
                .padding(.vertical, 16)
            }
            .plantyPage()
            .navigationTitle("Add a photo")
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
            // Not the whole-plant framing Snap asks for: this exists for the
            // turn where Planty asked to see one specific thing up close.
            CameraStage(
                camera: acquisition.camera,
                photoItem: photoItemBinding,
                guidance: "Frame whatever Planty asked to see.",
                footnote: "A close-up is fine. One photo per message.",
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
