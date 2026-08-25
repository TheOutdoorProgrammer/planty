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
                    #if DEBUG
                    // A screenshot route must not be covered by a permission sheet.
                    guard ProcessInfo.processInfo.environment["PLANTY_START_SETTINGS_ROUTE"] == nil else { return }
                    #endif
                    guard session.configuration.isConfigured else { return }
                    await session.startPushNotifications()
                }
        }
    }
}
