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
        .overlay(alignment: .bottom) { caption }
        .statusBarHidden()
    }

    @ViewBuilder
    private var picture: some View {
        if let localJPEG, let image = UIImage(data: localJPEG) {
            Image(uiImage: image).resizable().scaledToFit()
        } else if let url = photo?.url ?? plant.photoURL {
            AsyncImage(url: url) { phase in
                switch phase {
                case .success(let image):
                    image.resizable().scaledToFit()
                case .failure:
                    unavailable
                default:
                    ProgressView().tint(PlantyColor.foreground)
                }
            }
        } else {
            unavailable
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
