import SwiftUI

/// One photograph, filling the screen, pinchable. A plant page shows it 200
/// points high, too small to see what the app is for: whether the undersides
/// have mites.
struct PhotoViewer: View {
    let plant: Plant
    let photo: Photo?
    let localJPEG: Data?

    @Environment(\.dismiss) private var dismiss
    @State private var zoom: CGFloat = 1
    @State private var settled: CGFloat = 1

    /// Held so it can be saved or shared. A remote photo is behind a presigned
    /// link that expires, so keeping the bytes once they arrive is the only way
    /// to hand them anywhere else.
    @State private var loaded: UIImage?
    @State private var saveResult: String?

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
                    withAnimation(.snappy) {
                        zoom = zoom > 1 ? 1 : 3
                        settled = zoom
                    }
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
            ProgressView().tint(PlantyColor.foreground)
        }
    }

    private var source: URL? { photo?.url ?? plant.photoURL }

    /// Fetched once and kept, rather than left to AsyncImage: the same bytes
    /// have to reach the share sheet and the photo library.
    private func gather() async {
        if let localJPEG {
            loaded = UIImage(data: localJPEG)
            return
        }
        guard let source else { return }
        guard let (data, _) = try? await URLSession.shared.data(from: source) else { return }
        loaded = UIImage(data: data)
    }

    /// Saving and sharing sit together: one keeps it, the other sends it on.
    @ViewBuilder
    private var keepButton: some View {
        if let loaded {
            HStack(spacing: 10) {
                Button {
                    UIImageWriteToSavedPhotosAlbum(loaded, nil, nil, nil)
                    withAnimation { saveResult = "Saved to Photos" }
                } label: {
                    Image(systemName: "arrow.down.circle")
                        .font(.headline)
                        .foregroundStyle(PlantyColor.foreground)
                        .frame(width: 44, height: 44)
                        .background(.black.opacity(0.5), in: Circle())
                }
                .accessibilityLabel("Save to Photos")

                ShareLink(item: Image(uiImage: loaded), preview: .init("Photo", image: Image(uiImage: loaded))) {
                    Image(systemName: "square.and.arrow.up")
                        .font(.headline)
                        .foregroundStyle(PlantyColor.foreground)
                        .frame(width: 44, height: 44)
                        .background(.black.opacity(0.5), in: Circle())
                }
                .accessibilityLabel("Share")

                if let saveResult {
                    Text(saveResult)
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(PlantyColor.foreground)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 6)
                        .background(.black.opacity(0.5), in: Capsule())
                        .task {
                            try? await Task.sleep(for: .seconds(2))
                            withAnimation { self.saveResult = nil }
                        }
                }
            }
            .padding(16)
        }
    }

    /// A presigned link expires, so this is ordinary rather than an error.
    private var unavailable: some View {
        VStack(spacing: 10) {
            Image(systemName: "photo").font(.largeTitle)
            Text("That photograph could not be loaded.")
                .font(.subheadline)
        }
        .foregroundStyle(PlantyColor.secondaryText)
    }

    private var closeButton: some View {
        Button {
            dismiss()
        } label: {
            Image(systemName: "xmark")
                .font(.headline)
                .foregroundStyle(PlantyColor.foreground)
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
            .foregroundStyle(PlantyColor.foreground)
            .padding(.horizontal, 14)
            .padding(.vertical, 8)
            .background(.black.opacity(0.5), in: Capsule())
            .padding(.bottom, 30)
        }
    }
}
