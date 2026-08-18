import SwiftUI

/// Three tabs, Snap in the middle. Diagnosis is deliberately not one of them:
/// it only makes sense with a plant and a photo already in hand.
struct RootTabView: View {
    @Environment(AppSession.self) private var session

    var body: some View {
        @Bindable var session = session

        TabView(selection: $session.selectedTab) {
            Tab("Today", systemImage: "sun.max.fill", value: AppTab.today) {
                TodayScreen()
            }
            Tab("Snap", systemImage: "camera.fill", value: AppTab.snap) {
                SnapScreen()
            }
            Tab("Plants", systemImage: "leaf.fill", value: AppTab.plants) {
                PlantsLibraryScreen()
            }
        }
        .tint(PlantyColor.pink)
    }
}
