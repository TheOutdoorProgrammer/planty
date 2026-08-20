import SwiftUI

extension View {
    func plantyCard(
        border: Color = PlantyColor.quietDecoration.opacity(0.16),
        padding: CGFloat = 16
    ) -> some View {
        self
            .padding(padding)
            .background(
                PlantyColor.surface,
                in: RoundedRectangle(cornerRadius: 18, style: .continuous)
            )
            .overlay {
                RoundedRectangle(cornerRadius: 18, style: .continuous)
                    .stroke(border, lineWidth: 1)
            }
            .shadow(color: Color.black.opacity(0.035), radius: 8, y: 3)
    }

    func plantyPage() -> some View {
        self
            .foregroundStyle(PlantyColor.foreground)
            .background(PlantyColor.background.ignoresSafeArea())
            .scrollContentBackground(.hidden)
    }
}

struct Eyebrow: View {
    let text: String
    var color = PlantyColor.green

    var body: some View {
        Text(text.uppercased())
            .font(.caption2.weight(.bold))
            .tracking(0.8)
            .foregroundStyle(color)
    }
}

/// Purple, named, and permanent. It never brings red chrome, a countdown or a
/// warning triangle with it: ownership is responsibility, not an emergency.
struct OwnershipBadge: View {
    let plant: Plant
    var alwaysShow = false

    var body: some View {
        if plant.isFriends || alwaysShow {
            Label(plant.ownershipLabel, systemImage: plant.isFriends ? "person.2.fill" : "person.fill")
                .font(.caption.weight(.semibold))
                .foregroundStyle(plant.isFriends ? PlantyColor.purple : PlantyColor.secondaryText)
                .padding(.horizontal, 9)
                .padding(.vertical, 5)
                .background(
                    (plant.isFriends ? PlantyColor.purple : PlantyColor.secondaryText).opacity(0.12),
                    in: Capsule()
                )
                .accessibilityLabel(plant.ownershipAccessibilityLabel)
        }
    }
}

/// Status is always words plus an icon, never colour alone.
struct StatusPill: View {
    let state: CareState

    var body: some View {
        Label(state.label, systemImage: state.symbol)
            .font(.caption.weight(.semibold))
            .foregroundStyle(state.color)
            .padding(.horizontal, 9)
            .padding(.vertical, 5)
            .background(state.color.opacity(0.11), in: Capsule())
            .accessibilityLabel("\(state.label). \(state.sentence)")
    }
}

struct PrimaryButtonStyle: ButtonStyle {
    var color = PlantyColor.green

    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.headline)
            .foregroundStyle(PlantyColor.background)
            .frame(maxWidth: .infinity, minHeight: 52)
            .padding(.horizontal, 16)
            .background(
                color.opacity(configuration.isPressed ? 0.78 : 1),
                in: RoundedRectangle(cornerRadius: 15, style: .continuous)
            )
            .contentShape(RoundedRectangle(cornerRadius: 15, style: .continuous))
            .scaleEffect(configuration.isPressed ? 0.985 : 1)
    }
}

struct SecondaryButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.headline)
            .foregroundStyle(PlantyColor.foreground)
            .frame(maxWidth: .infinity, minHeight: 50)
            .padding(.horizontal, 14)
            .background(
                PlantyColor.surface.opacity(configuration.isPressed ? 0.72 : 1),
                in: RoundedRectangle(cornerRadius: 15, style: .continuous)
            )
            .overlay {
                RoundedRectangle(cornerRadius: 15, style: .continuous)
                    .stroke(PlantyColor.quietDecoration.opacity(0.2), lineWidth: 1)
            }
            .contentShape(RoundedRectangle(cornerRadius: 15, style: .continuous))
            .scaleEffect(configuration.isPressed ? 0.985 : 1)
    }
}

/// A headline plus body plus buttons, which is the shape of nearly every state
/// screen. The icon and title share one line so routine messages do not feel
/// like full-screen alerts.
struct StateMessage<Actions: View>: View {
    let title: String
    let message: String
    var accent: Color = PlantyColor.foreground
    var icon: String?
    @ViewBuilder var actions: Actions

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            HStack(alignment: .firstTextBaseline, spacing: 10) {
                if let icon {
                    Image(systemName: icon)
                        .font(.headline.weight(.semibold))
                        .foregroundStyle(accent)
                        .accessibilityHidden(true)
                }
                Text(title)
                    .font(.title3.weight(.bold))
                    .foregroundStyle(PlantyColor.foreground)
            }
            Text(message)
                .font(.body)
                .foregroundStyle(PlantyColor.secondaryText)
            actions
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: accent.opacity(0.22))
    }
}

struct SaveToast: View {
    let message: String

    var body: some View {
        Label(message, systemImage: "checkmark.circle.fill")
            .font(.subheadline.weight(.semibold))
            .foregroundStyle(PlantyColor.background)
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
            .background(PlantyColor.green, in: Capsule())
            .accessibilityAddTraits(.isStaticText)
    }
}

struct SectionHeading: View {
    let text: String
    var detail: String?

    init(text: String, detail: String? = nil) {
        self.text = text
        self.detail = detail
    }

    init(_ text: String, detail: String? = nil) {
        self.init(text: text, detail: detail)
    }

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(text)
                .font(.title3.weight(.bold))
                .foregroundStyle(PlantyColor.foreground)
            if let detail {
                Text(detail)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }
}

struct DestructiveButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.headline)
            .foregroundStyle(PlantyColor.red)
            .frame(maxWidth: .infinity, minHeight: 50)
            .padding(.horizontal, 14)
            .background(
                PlantyColor.red.opacity(configuration.isPressed ? 0.16 : 0.08),
                in: RoundedRectangle(cornerRadius: 15, style: .continuous)
            )
            .overlay {
                RoundedRectangle(cornerRadius: 15, style: .continuous)
                    .stroke(PlantyColor.red.opacity(0.24), lineWidth: 1)
            }
            .contentShape(RoundedRectangle(cornerRadius: 15, style: .continuous))
    }
}

/// Progress is only claimed when it is true; no rotating fake status phrases.
struct ThinkingRow: View {
    let stageLine: String?

    var body: some View {
        HStack(spacing: 10) {
            ProgressView().tint(PlantyColor.cyan)
            VStack(alignment: .leading, spacing: 2) {
                Text("Looking closer…")
                    .font(.subheadline.weight(.semibold))
                if let stageLine {
                    Text(stageLine)
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(padding: 14)
        .accessibilityElement(children: .combine)
    }
}

/// A compact face for navigation and action tiles. Text leads; the icon helps
/// scanning but never becomes the only explanation of what a control does.
struct ActionFace: View {
    let title: String
    let icon: String
    var detail: String?

    init(_ title: String, icon: String, detail: String? = nil) {
        self.title = title
        self.icon = icon
        self.detail = detail
    }

    var body: some View {
        HStack(spacing: 10) {
            Image(systemName: icon)
                .font(.body.weight(.semibold))
                .frame(width: 24)
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.subheadline.weight(.semibold))
                    .lineLimit(2)
                if let detail {
                    Text(detail)
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                        .lineLimit(2)
                }
            }
            Spacer(minLength: 0)
        }
        .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
        .accessibilityElement(children: .combine)
    }
}

extension String {
    var cleaned: String { trimmingCharacters(in: .whitespacesAndNewlines) }

    var nilIfBlank: String? {
        let trimmed = cleaned
        return trimmed.isEmpty ? nil : trimmed
    }
}
