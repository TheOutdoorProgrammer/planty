import Photos
import SwiftUI

/// One photograph, filling the screen, pinchable. A plant page shows it 200
/// points high, too small to see what the app is for: whether the undersides
/// have mites.
struct PhotoViewer: View {
    let plant: Plant
    let photo: Photo?
    let localJPEG: Data?

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var zoom: CGFloat = 1
    @State private var settled: CGFloat = 1

    /// Held so it can be saved or shared. A remote photo is behind a presigned
    /// link that expires, so keeping the bytes once they arrive is the only way
    /// to hand them anywhere else.
    @State private var loaded: UIImage?
    @State private var saveResult: PhotoSaveFeedback?
    @State private var isSaving = false

    var body: some View {
        ZStack {
            Color.black.ignoresSafeArea()

            picture
                .scaleEffect(zoom)
                .gesture(
                    MagnifyGesture()
                        .onChanged { zoom = min(max(settled * $0.magnification, 1), 6) }
                        .onEnded { _ in settled = zoom }
                )
                .onTapGesture(count: 2) {
                    zoom = zoom > 1 ? 1 : 3
                    settled = zoom
                }
        }
        .overlay(alignment: .topTrailing) { closeButton }
        .overlay(alignment: .topLeading) { keepButton }
        .overlay(alignment: .bottom) { caption }
        .statusBarHidden()
        .task { await gather() }
    }

    @ViewBuilder
    private var picture: some View {
        if let loaded {
            Image(uiImage: loaded).resizable().scaledToFit()
        } else if source == nil {
            unavailable
        } else {
            ProgressView().tint(.white)
        }
    }

    private var source: URL? { photo?.renderableURL ?? plant.renderablePhotoURL }

    /// Fetched once and kept, rather than left to AsyncImage: the same bytes
    /// have to reach the share sheet and the photo library.
    private func gather() async {
        if let localJPEG {
            loaded = UIImage(data: localJPEG)
            if let photo {
                await session.images.seed(
                    localJPEG,
                    for: .photo(photo, rendition: .original)
                )
            }
            return
        }
        guard let source else { return }
        let key = photo.map { ImageCacheKey.photo($0, rendition: .original) }
            ?? ImageCacheKey.cover(plant, rendition: .original)
        guard let data = try? await session.images.data(for: key, from: source),
              !Task.isCancelled
        else { return }
        loaded = UIImage(data: data)
    }

    /// Saving and sharing sit together: one keeps it, the other sends it on.
    @ViewBuilder
    private var keepButton: some View {
        if let loaded {
            HStack(spacing: 10) {
                Button {
                    Task { await saveToPhotos(loaded) }
                } label: {
                    Group {
                        if isSaving {
                            ProgressView().tint(.white)
                        } else {
                            Image(systemName: "arrow.down.circle")
                                .font(.headline)
                                .foregroundStyle(.white)
                        }
                    }
                    .frame(width: 44, height: 44)
                    .background(.black.opacity(0.5), in: Circle())
                }
                .disabled(isSaving)
                .accessibilityLabel(isSaving ? "Saving to Photos" : "Save to Photos")

                ShareLink(item: Image(uiImage: loaded), preview: .init("Photo", image: Image(uiImage: loaded))) {
                    Image(systemName: "square.and.arrow.up")
                        .font(.headline)
                        .foregroundStyle(.white)
                        .frame(width: 44, height: 44)
                        .background(.black.opacity(0.5), in: Circle())
                }
                .accessibilityLabel("Share")

                if let saveResult {
                    Text(saveResult.message)
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(saveResult == .saved
                            ? Color.white : PlantyColor.orange)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 6)
                        .background(.black.opacity(0.5), in: Capsule())
                        .task {
                            try? await Task.sleep(for: .seconds(2))
                            self.saveResult = nil
                        }
                }
            }
            .padding(16)
        }
    }

    private func saveToPhotos(_ image: UIImage) async {
        guard !isSaving else { return }
        isSaving = true
        defer { isSaving = false }

        do {
            try await PHPhotoLibrary.shared().performChanges {
                PHAssetChangeRequest.creationRequestForAsset(from: image)
            }
            saveResult = .saved
        } catch {
            saveResult = .failed
        }
    }

    /// A presigned link expires, so this is ordinary rather than an error.
    private var unavailable: some View {
        VStack(spacing: 10) {
            Image(systemName: "photo").font(.largeTitle)
            Text("That photograph could not be loaded.")
                .font(.subheadline)
        }
        .foregroundStyle(.white.opacity(0.75))
    }

    private var closeButton: some View {
        Button {
            dismiss()
        } label: {
            Image(systemName: "xmark")
                .font(.headline)
                .foregroundStyle(.white)
                .frame(width: 44, height: 44)
                .background(.black.opacity(0.5), in: Circle())
        }
        .padding(16)
        .accessibilityLabel("Close")
    }

    @ViewBuilder
    private var caption: some View {
        if let photo {
            VStack(spacing: 2) {
                Text(photo.takenAt.formatted(date: .abbreviated, time: .shortened))
                    .font(.footnote.weight(.semibold))
                if let words = photo.caption, !words.isEmpty {
                    Text(words).font(.caption)
                }
            }
            .foregroundStyle(.white)
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
            .background(.black.opacity(0.5), in: Capsule())
            .padding(.bottom, 30)
        }
    }
}

private enum PhotoSaveFeedback: Equatable {
    case saved
    case failed

    var message: String {
        switch self {
        case .saved: "Saved to Photos"
        case .failed: "Could not save to Photos"
        }
    }
}
