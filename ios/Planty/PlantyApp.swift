import SwiftUI

@main
struct PlantyApp: App {
    @UIApplicationDelegateAdaptor(PlantyAppDelegate.self) private var appDelegate
    @State private var session = AppSession()

    var body: some Scene {
        WindowGroup {
            RootTabView()
                .environment(session)
                .tint(PlantyColor.pink)
                .task(id: session.configuration.baseURL) {
                    guard session.configuration.isConfigured else { return }
                    await session.startPushNotifications()
                }
        }
    }
}
