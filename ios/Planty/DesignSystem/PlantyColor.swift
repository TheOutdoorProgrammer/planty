import SwiftUI

/// Dracula Classic, used semantically. See design/UI-CONCEPT.md for the roles.
enum PlantyColor {
    static let background = Color(hex: 0x282A36)
    static let surface = Color(hex: 0x44475A)
    static let foreground = Color(hex: 0xF8F8F2)

    /// Dracula comment blue. Strokes and dividers only. It fails contrast for
    /// small text on the canvas, so it is never a foregroundStyle.
    static let quietDecoration = Color(hex: 0x6272A4)

    /// What secondary copy uses instead of comment blue.
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
