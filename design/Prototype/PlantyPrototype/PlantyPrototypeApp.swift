import SwiftUI

@main
struct PlantyPrototypeApp: App {
    var body: some Scene {
        WindowGroup {
            PlantyRootView()
                .preferredColorScheme(.dark)
        }
    }
}

struct PlantyRootView: View {
    @State private var selectedTab = AppTab.today

    var body: some View {
        TabView(selection: $selectedTab) {
            TodayView {
                selectedTab = .snap
            }
            .tabItem {
                Label("Today", systemImage: "sun.max.fill")
            }
            .tag(AppTab.today)

            NavigationStack {
                CaptureFlowView()
            }
            .tabItem {
                Label("Snap", systemImage: "camera.fill")
            }
            .tag(AppTab.snap)

            NavigationStack {
                PlantStoryView(plant: .mona) {
                    selectedTab = .snap
                }
            }
            .tabItem {
                Label("Plants", systemImage: "leaf.fill")
            }
            .tag(AppTab.plants)
        }
        .tint(PlantyColor.pink)
    }
}

private enum AppTab: Hashable {
    case today
    case snap
    case plants
}
