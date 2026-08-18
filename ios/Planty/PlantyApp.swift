import SwiftUI

@main
struct PlantyApp: App {
    @State private var session = AppSession()

    var body: some Scene {
        WindowGroup {
            RootTabView()
                .environment(session)
                .preferredColorScheme(.dark)
                .tint(PlantyColor.pink)
        }
    }
}
