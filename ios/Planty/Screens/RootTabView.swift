import SwiftUI

/// The tabs use task names rather than internal product concepts. The existing
/// Garden feature remains intact behind More; Capture says what Snap actually
/// does and keeps the camera one tap away from everywhere.
struct RootTabView: View {
    @Environment(AppSession.self) private var session

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
            SettingsScreen()
        }
        .onReceive(NotificationCenter.default.publisher(for: .plantyPushOpened)) { notification in
            guard let route = notification.object as? PlantyPushRoute else { return }
            session.openPushRoute(route)
        }
    }
}
