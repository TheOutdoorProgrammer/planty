import SwiftUI

enum PlantOwnership: Hashable {
    case friend
    case mine

    var label: String {
        switch self {
        case .friend: "Maya's plant"
        case .mine: "Mine"
        }
    }

    var icon: String {
        switch self {
        case .friend: "person.2.fill"
        case .mine: "person.fill"
        }
    }

    var accessibilityLabel: String {
        switch self {
        case .friend: "Friend's plant, belongs to Maya"
        case .mine: "My plant"
        }
    }
}

struct Plant: Identifiable, Hashable {
    let id: UUID
    let name: String
    let species: String
    let room: String
    let ownership: PlantOwnership
}

extension Plant {
    static let mona = Plant(
        id: UUID(uuidString: "0651DE3F-6EC5-4A7B-9981-8CF8F53D0F4D")!,
        name: "Mona",
        species: "Swiss cheese plant",
        room: "Living room",
        ownership: .friend
    )

    static let fern = Plant(
        id: UUID(uuidString: "5E0D5C1E-D947-43B2-98D1-76E4A7EC83F7")!,
        name: "Fernie Sanders",
        species: "Boston fern",
        room: "Greenhouse cabinet",
        ownership: .mine
    )
}

enum PhotoMoment: Hashable {
    case today
    case augustEleven
    case julyTwenty
    case camera

    var label: String {
        switch self {
        case .today: "Today, 8:03 AM"
        case .augustEleven: "August 11"
        case .julyTwenty: "July 20"
        case .camera: "Live preview"
        }
    }

    var symbol: String {
        switch self {
        case .today: "leaf.fill"
        case .augustEleven: "leaf.circle.fill"
        case .julyTwenty: "camera.macro"
        case .camera: "viewfinder"
        }
    }

    var rotation: Double {
        switch self {
        case .today: -7
        case .augustEleven: 9
        case .julyTwenty: -15
        case .camera: 0
        }
    }

    var gradient: [Color] {
        switch self {
        case .today:
            [Color(hex: 0x35534B), Color(hex: 0x1F2D2A), PlantyColor.surface]
        case .augustEleven:
            [Color(hex: 0x594A55), Color(hex: 0x2E403A), PlantyColor.background]
        case .julyTwenty:
            [Color(hex: 0x574B3B), Color(hex: 0x2E3734), PlantyColor.background]
        case .camera:
            [Color(hex: 0x394B43), Color(hex: 0x26332F), PlantyColor.background]
        }
    }

    func accessibilityDescription(for plant: Plant) -> String {
        switch self {
        case .today:
            "Photo of \(plant.name) today from the front; two new leaves are opening and lower leaf yellowing is unchanged."
        case .augustEleven:
            "Photo of \(plant.name) on August 11; lower yellowing is visible but no longer spreading."
        case .julyTwenty:
            "Photo of \(plant.name) on July 20; first visible yellowing on the lower leaves."
        case .camera:
            "Mock live camera preview for \(plant.name)."
        }
    }
}

struct StoryEvent: Identifiable {
    let id = UUID()
    let date: String
    let title: String
    let narrative: String
    let moment: PhotoMoment
    let accent: Color
}

extension StoryEvent {
    static let monaStory: [StoryEvent] = [
        StoryEvent(
            date: "AUG 18",
            title: "New growth",
            narrative: "Two leaves are opening. The yellowing has not spread. No action needed.",
            moment: .today,
            accent: PlantyColor.green
        ),
        StoryEvent(
            date: "AUG 11",
            title: "Holding steady",
            narrative: "Yellowing stopped spreading after you emptied the tray on August 8.",
            moment: .augustEleven,
            accent: PlantyColor.cyan
        ),
        StoryEvent(
            date: "JUL 20",
            title: "First change noticed",
            narrative: "Lower leaves began yellowing. Moisture had stayed high for most of the week.",
            moment: .julyTwenty,
            accent: PlantyColor.yellow
        )
    ]
}
