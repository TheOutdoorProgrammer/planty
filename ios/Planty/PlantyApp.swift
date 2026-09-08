import SwiftUI

@main
struct PlantyApp: App {
    @UIApplicationDelegateAdaptor(PlantyAppDelegate.self) private var appDelegate
    @State private var session = AppSession()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            RootTabView()
                .environment(session)
                .tint(PlantyColor.pink)
                .onChange(of: scenePhase) { _, phase in
                    if phase == .active {
                        PlantyTelemetry.shared.resumeDelivery()
                    } else if phase == .background {
                        PlantyTelemetry.shared.pauseDelivery()
                    }
                }
                .task(id: session.configuration.baseURL) {
                    PlantyTelemetry.shared.configure(session.configuration)
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
