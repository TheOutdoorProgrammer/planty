import Foundation

/// One thing the model did on the way to an answer. Shown collapsed, since "I
/// looked it up" is worth nothing without which page, and a wall of transcript
/// is worth nothing either.
struct AnswerStep: Codable, Sendable, Hashable, Identifiable {
    enum Kind: String, Codable, Sendable {
        case thought
        case action

        /// An unrecognised kind reads as an action, since a step the app has
        /// never heard of is still something that happened.
        init(from decoder: Decoder) throws {
            let raw = try decoder.singleValueContainer().decode(String.self)
            self = Kind(rawValue: raw) ?? .action
        }
    }

    let kind: Kind
    let tool: String?
    let detail: String?
    let output: String?

    /// Steps have no server id and a conversation can repeat a command, so
    /// identity is the whole content.
    var id: Int { hashValue }

    enum CodingKeys: String, CodingKey {
        case kind, tool, detail, output
    }

    /// What the row says before it is opened.
    var headline: String {
        switch kind {
        case .thought:
            return "Thought about it"
        case .action:
            switch tool {
            case "Bash": return "Ran a command"
            case "WebFetch": return "Read a page"
            case "WebSearch": return "Searched the web"
            case "Read": return "Opened a photograph"
            case .some(let other) where !other.isEmpty: return other
            default: return "Did something"
            }
        }
    }

    /// The one line worth seeing without opening it: the command, the URL.
    var subtitle: String? {
        guard kind == .action, let detail, !detail.isEmpty else { return nil }
        return detail
    }

    /// A step whose command was refused. Worth marking: a refusal explains an
    /// answer that would otherwise look like the model simply chose not to.
    var refused: Bool { output?.hasPrefix("failed:") ?? false }

    var icon: String {
        if refused { return "exclamationmark.triangle" }
        switch kind {
        case .thought: return "bubble.left.and.text.bubble.right"
        case .action:
            switch tool {
            case "Bash": return "terminal"
            case "WebFetch", "WebSearch": return "globe"
            case "Read": return "photo"
            default: return "gearshape"
            }
        }
    }

    /// The body shown when opened, which for a thought is the thought itself.
    var body: String? {
        switch kind {
        case .thought: return output
        case .action: return output
        }
    }
}
