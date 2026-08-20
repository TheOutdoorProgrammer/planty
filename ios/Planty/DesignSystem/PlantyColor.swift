import SwiftUI

/// Planty's palette follows the system appearance instead of forcing the app
/// to look like a developer theme. Brand colour is reserved for meaning:
/// green is healthy/calm, pink is capture, and the remaining colours are
/// semantic accents rather than decoration.
enum PlantyColor {
    static let background = Color(uiColor: .systemGroupedBackground)
    static let surface = Color(uiColor: .secondarySystemGroupedBackground)
    static let elevated = Color(uiColor: .tertiarySystemGroupedBackground)
    static let foreground = Color.primary
    static let secondaryText = Color.secondary
    static let quietDecoration = Color(uiColor: .separator)

    static let green = Color(uiColor: .systemGreen)
    static let cyan = Color(uiColor: .systemTeal)
    static let orange = Color(uiColor: .systemOrange)
    static let pink = Color(uiColor: .systemPink)
    static let purple = Color(uiColor: .systemPurple)
    static let red = Color(uiColor: .systemRed)
    static let yellow = Color(uiColor: .systemYellow)
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
