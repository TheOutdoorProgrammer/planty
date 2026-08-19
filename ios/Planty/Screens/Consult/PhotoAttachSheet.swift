import PhotosUI
import SwiftUI

/// One photograph for the message being written. Reuses Snap's camera stage,
/// so denied permission and camera-less simulators behave the same here.
struct PhotoAttachSheet: View {
    let attach: (Data) -> Void

    @Environment(\.dismiss) private var dismiss
    @State private var camera = CameraController()
    @State private var photoItem: PhotosPickerItem?

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(spacing: 18) {
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
            .task { await camera.prepare() }
            .onDisappear { camera.stop() }
            .onChange(of: photoItem) { _, item in
                Task { await load(item) }
            }
        }
    }

    @ViewBuilder
    private var stage: some View {
        switch camera.availability {
        case .denied:
            CameraPermissionCard(photoItem: $photoItem)
        case .unavailable:
            NoCameraCard(photoItem: $photoItem)
        case .ready, .unknown:
            // Not the whole-plant framing Snap asks for: this exists for the
            // turn where Planty asked to see one specific thing up close.
            CameraStage(
                camera: camera,
                photoItem: $photoItem,
                guidance: "Frame whatever Planty asked to see.",
                footnote: "A close-up is fine. One photo per message.",
                shutter: { Task { await shoot() } }
            )
        }
    }

    private func shoot() async {
        guard let jpeg = try? await camera.capture() else { return }
        attach(jpeg)
        dismiss()
    }

    private func load(_ item: PhotosPickerItem?) async {
        guard let item,
              let data = try? await item.loadTransferable(type: Data.self)
        else { return }
        attach(data)
        dismiss()
    }
}
