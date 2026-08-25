import AppIntents

struct StartPlantCareRoundIntent: AppIntent {
    static let title: LocalizedStringResource = "Start Plant Care Round"
    static let description = IntentDescription("Open due plant care grouped by room.")
    static let openAppWhenRun = true

    @MainActor
    func perform() async throws -> some IntentResult & OpensIntent {
        .result(opensIntent: OpenURLIntent(PlantDeepLink.careRoundURL))
    }
}

struct PlantyAppShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(
            intent: StartPlantCareRoundIntent(),
            phrases: [
                "Start a care round in \(.applicationName)",
                "What plants need care in \(.applicationName)"
            ],
            shortTitle: "Plant care round",
            systemImageName: "leaf.circle.fill"
        )
    }
}
