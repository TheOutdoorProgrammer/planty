import SwiftUI

extension View {
    func plantyCard(
        border: Color = PlantyColor.quietDecoration.opacity(0.45),
        padding: CGFloat = 18
    ) -> some View {
        self
            .padding(padding)
            .background(
                PlantyColor.surface,
                in: RoundedRectangle(cornerRadius: 24, style: .continuous)
            )
            .overlay {
                RoundedRectangle(cornerRadius: 24, style: .continuous)
                    .stroke(border, lineWidth: 1)
            }
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
    var color = PlantyColor.cyan

    var body: some View {
        Text(text.uppercased())
            .font(.caption.weight(.bold))
            .tracking(1.1)
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
                .padding(.horizontal, 10)
                .padding(.vertical, 7)
                .background(
                    (plant.isFriends ? PlantyColor.purple : PlantyColor.secondaryText).opacity(0.14),
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
            .font(.caption.weight(.bold))
            .foregroundStyle(state.color)
            .padding(.horizontal, 10)
            .padding(.vertical, 7)
            .background(state.color.opacity(0.14), in: Capsule())
            .accessibilityLabel("\(state.label). \(state.sentence)")
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
            .contentShape(Capsule())
    }
}

struct SecondaryButtonStyle: ButtonStyle {
    func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.headline)
            .foregroundStyle(PlantyColor.foreground)
            .frame(maxWidth: .infinity, minHeight: 50)
            .padding(.horizontal, 16)
            .background(
                PlantyColor.surface.opacity(configuration.isPressed ? 0.7 : 1),
                in: Capsule()
            )
            .overlay {
                Capsule().stroke(PlantyColor.quietDecoration.opacity(0.55), lineWidth: 1)
            }
            .contentShape(Capsule())
    }
}

/// A headline plus body plus buttons, which is the shape of nearly every state
/// screen in SCREENS.md.
struct StateMessage<Actions: View>: View {
    let title: String
    let message: String
    var accent: Color = PlantyColor.foreground
    var icon: String?
    @ViewBuilder var actions: Actions

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            if let icon {
                Image(systemName: icon)
                    .font(.title2.weight(.semibold))
                    .foregroundStyle(accent)
                    .accessibilityHidden(true)
            }
            Text(title)
                .font(.title2.weight(.bold))
                .foregroundStyle(accent)
            Text(message)
                .font(.body)
                .foregroundStyle(PlantyColor.secondaryText)
            actions
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: accent.opacity(0.4))
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

    var body: some View {
        Text(text)
            .font(.title3.weight(.bold))
            .foregroundStyle(PlantyColor.foreground)
            .frame(maxWidth: .infinity, alignment: .leading)
    }
}
