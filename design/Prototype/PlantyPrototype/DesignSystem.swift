import SwiftUI

enum PlantyColor {
    static let background = Color(hex: 0x282A36)
    static let surface = Color(hex: 0x44475A)
    static let foreground = Color(hex: 0xF8F8F2)
    static let quietDecoration = Color(hex: 0x6272A4)
    static let secondaryText = Color(hex: 0xC8CAE0)
    static let green = Color(hex: 0x50FA7B)
    static let cyan = Color(hex: 0x8BE9FD)
    static let orange = Color(hex: 0xFFB86C)
    static let pink = Color(hex: 0xFF79C6)
    static let purple = Color(hex: 0xBD93F9)
    static let red = Color(hex: 0xFF5555)
    static let yellow = Color(hex: 0xF1FA8C)
}

extension Color {
    init(hex: UInt32, opacity: Double = 1) {
        self.init(
            .sRGB,
            red: Double((hex >> 16) & 0xFF) / 255,
            green: Double((hex >> 8) & 0xFF) / 255,
            blue: Double(hex & 0xFF) / 255,
            opacity: opacity
        )
    }
}

extension View {
    func plantyCard(
        border: Color = PlantyColor.quietDecoration.opacity(0.45),
        padding: CGFloat = 18
    ) -> some View {
        self
            .padding(padding)
            .background(PlantyColor.surface, in: RoundedRectangle(cornerRadius: 24, style: .continuous))
            .overlay {
                RoundedRectangle(cornerRadius: 24, style: .continuous)
                    .stroke(border, lineWidth: 1)
            }
    }

    func pageBackground() -> some View {
        self
            .foregroundStyle(PlantyColor.foreground)
            .background(PlantyColor.background.ignoresSafeArea())
    }
}

struct PlantyLogo: View {
    var size: CGFloat = 132

    var body: some View {
        Image("planty-mark")
            .resizable()
            .scaledToFit()
            .frame(width: size, height: size)
            .accessibilityLabel("Planty, a delighted pink starfish holding a crooked potted plant")
    }
}

struct Eyebrow: View {
    let text: String
    var color = PlantyColor.cyan

    var body: some View {
        Text(text.uppercased())
            .font(.caption.weight(.bold))
            .tracking(1.1)
            .foregroundStyle(color)
    }
}

struct OwnershipBadge: View {
    let ownership: PlantOwnership

    var body: some View {
        Label(ownership.label, systemImage: ownership.icon)
            .font(.caption.weight(.semibold))
            .foregroundStyle(ownership == .friend ? PlantyColor.purple : PlantyColor.secondaryText)
            .padding(.horizontal, 10)
            .padding(.vertical, 7)
            .background(
                (ownership == .friend ? PlantyColor.purple : PlantyColor.secondaryText).opacity(0.12),
                in: Capsule()
            )
            .accessibilityLabel(ownership.accessibilityLabel)
    }
}

struct StatusPill: View {
    let title: String
    let icon: String
    let color: Color

    var body: some View {
        Label(title, systemImage: icon)
            .font(.caption.weight(.bold))
            .foregroundStyle(color)
            .padding(.horizontal, 10)
            .padding(.vertical, 7)
            .background(color.opacity(0.13), in: Capsule())
    }
}

struct PrimaryButtonStyle: ButtonStyle {
    var color = PlantyColor.pink

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.headline)
            .foregroundStyle(PlantyColor.background)
            .frame(maxWidth: .infinity, minHeight: 52)
            .padding(.horizontal, 18)
            .background(color.opacity(configuration.isPressed ? 0.78 : 1), in: Capsule())
            .scaleEffect(configuration.isPressed ? 0.98 : 1)
    }
}

struct SecondaryButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.headline)
            .foregroundStyle(PlantyColor.foreground)
            .frame(maxWidth: .infinity, minHeight: 50)
            .padding(.horizontal, 16)
            .background(PlantyColor.surface.opacity(configuration.isPressed ? 0.7 : 1), in: Capsule())
            .overlay {
                Capsule().stroke(PlantyColor.quietDecoration.opacity(0.55), lineWidth: 1)
            }
    }
}

struct PlantPhoto: View {
    let plant: Plant
    let moment: PhotoMoment
    var height: CGFloat = 220

    var body: some View {
        ZStack {
            LinearGradient(
                colors: moment.gradient,
                startPoint: .topLeading,
                endPoint: .bottomTrailing
            )

            RoundedRectangle(cornerRadius: 90, style: .continuous)
                .fill(PlantyColor.background.opacity(0.24))
                .frame(width: 180, height: 210)
                .rotationEffect(.degrees(-12))

            Image(systemName: moment.symbol)
                .font(.system(size: 116, weight: .light))
                .symbolRenderingMode(.palette)
                .foregroundStyle(PlantyColor.green, PlantyColor.background.opacity(0.72))
                .rotationEffect(.degrees(moment.rotation))

            VStack {
                Spacer()
                HStack {
                    Label(moment.label, systemImage: "camera.aperture")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(PlantyColor.foreground)
                        .padding(.horizontal, 10)
                        .padding(.vertical, 7)
                        .background(PlantyColor.background.opacity(0.76), in: Capsule())
                    Spacer()
                }
                .padding(14)
            }
        }
        .frame(maxWidth: .infinity)
        .frame(height: height)
        .clipShape(RoundedRectangle(cornerRadius: 24, style: .continuous))
        .overlay {
            RoundedRectangle(cornerRadius: 24, style: .continuous)
                .stroke(PlantyColor.foreground.opacity(0.12), lineWidth: 1)
        }
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(moment.accessibilityDescription(for: plant))
    }
}

struct QuickActionRow: View {
    let title: String
    let subtitle: String
    let icon: String
    var color = PlantyColor.foreground

    var body: some View {
        HStack(spacing: 14) {
            Image(systemName: icon)
                .font(.title3.weight(.semibold))
                .foregroundStyle(color)
                .frame(width: 36, height: 36)
                .background(color.opacity(0.13), in: Circle())

            VStack(alignment: .leading, spacing: 3) {
                Text(title)
                    .font(.headline)
                    .foregroundStyle(PlantyColor.foreground)
                Text(subtitle)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            Spacer(minLength: 8)

            Image(systemName: "chevron.right")
                .font(.subheadline.weight(.bold))
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .frame(maxWidth: .infinity, minHeight: 56)
        .contentShape(Rectangle())
    }
}
