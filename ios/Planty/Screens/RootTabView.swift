import SwiftUI

/// The tabs use task names rather than internal product concepts. The existing
/// Garden feature remains intact behind More; Capture says what Snap actually
/// does and keeps the camera one tap away from everywhere.
struct RootTabView: View {
    @Environment(AppSession.self) private var session
    @State private var pushRoutes = PushRouteCenter.shared

    var body: some View {
        @Bindable var session = session

        TabView(selection: $session.selectedTab) {
            Tab("Today", systemImage: "checkmark.circle.fill", value: AppTab.today) {
                TodayScreen()
            }
            Tab("Capture", systemImage: "camera.fill", value: AppTab.snap) {
                SnapScreen()
            }
            Tab("Plants", systemImage: "leaf.fill", value: AppTab.plants) {
                PlantsLibraryScreen()
            }
            Tab("More", systemImage: "square.grid.2x2.fill", value: AppTab.garden) {
                GardenScreen()
            }
        }
        .tint(PlantyColor.green)
        .tabViewStyle(.sidebarAdaptable)
        .sheet(isPresented: $session.isShowingSettings) {
            settingsDestination
        }
        .sheet(isPresented: $session.isShowingCareRound) {
            CareRoundScreen()
        }
        .onChange(of: pushRoutes.pending, initial: true) { _, pending in
            guard pending != nil, let route = pushRoutes.takePending() else { return }
            session.openPushRoute(route)
        }
        .onOpenURL { session.openDeepLink($0) }
    }

    @ViewBuilder
    private var settingsDestination: some View {
        #if DEBUG
        if ProcessInfo.processInfo.environment["PLANTY_START_SETTINGS_ROUTE"] == "policies" {
            NavigationStack { PolicySettingsScreen() }
        } else if ProcessInfo.processInfo.environment["PLANTY_START_SETTINGS_ROUTE"] == "scheduled-jobs" {
            NavigationStack { ScheduledJobsScreen() }
        } else if ProcessInfo.processInfo.environment["PLANTY_START_SETTINGS_ROUTE"] == "sensors" {
            NavigationStack { SensorListScreen(api: session.api) }
        } else {
            SettingsScreen()
        }
        #else
        SettingsScreen()
        #endif
    }
}
