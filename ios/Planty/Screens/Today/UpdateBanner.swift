import SwiftUI

/// Says a newer build exists and hands off to Fledge's install page. It cannot
/// install anything itself: an ad hoc build is installed by Safari, which is
/// why this links out rather than offering a button that would not work.
struct UpdateBanner: View {
    let release: FledgeRelease
    let dismiss: () -> Void

    @Environment(\.openURL) private var openURL

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack(spacing: 8) {
                Image(systemName: "arrow.down.circle.fill")
                    .foregroundStyle(PlantyColor.cyan)
                Text("Planty \(release.label) is ready")
                    .font(.subheadline.weight(.semibold))
                Spacer()
                Button {
                    dismiss()
                } label: {
                    Image(systemName: "xmark")
                        .font(.caption.weight(.bold))
                        .foregroundStyle(PlantyColor.secondaryText)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Dismiss update notice")
            }

            if let latest = release.notes.first?.notes, !latest.isEmpty {
                Text(latest)
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            Button("Open the install page") {
                openURL(release.installPageURL)
            }
            .buttonStyle(SecondaryButtonStyle())

            // Tapping Install in an embedded browser silently does nothing,
            // and somebody who does not know that concludes the build is bad.
            Text("Opens in Safari, which is the only browser that can install it.")
                .font(.caption2)
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(PlantyColor.surface, in: RoundedRectangle(cornerRadius: 16, style: .continuous))
        .overlay {
            RoundedRectangle(cornerRadius: 16, style: .continuous)
                .stroke(PlantyColor.cyan.opacity(0.4), lineWidth: 1)
        }
    }
}
