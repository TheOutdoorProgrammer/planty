import UIKit
import UserNotifications

/// Bridges UIApplication's device-token callbacks back into the same Planty API
/// configuration the rest of the app is using. The token is kept only long
/// enough to re-register it when Settings points the app at a different server.
@MainActor
final class PushRegistrationCenter {
    static let shared = PushRegistrationCenter()

    private var api: (any PlantyAPI)?
    private var token: String?

    private init() {}

    func configure(api: any PlantyAPI) {
        self.api = api
        if token != nil {
            Task { await syncToken() }
        }
    }

    func requestAuthorization() async {
        do {
            let granted = try await UNUserNotificationCenter.current().requestAuthorization(
                options: [.alert, .badge, .sound]
            )
            guard granted else { return }
            UIApplication.shared.registerForRemoteNotifications()
        } catch {
            // Permission state remains visible in iOS Settings. A notification
            // failure must never keep the rest of Planty from launching.
        }
    }

    func didRegister(deviceToken: Data) {
        token = deviceToken.map { String(format: "%02x", $0) }.joined()
        Task { await syncToken() }
    }

    private func syncToken() async {
        guard let token, let api else { return }
        let environment: String
        #if DEBUG
        environment = "sandbox"
        #else
        environment = "production"
        #endif
        try? await api.registerPushDevice(
            PushDeviceRegistration(token: token, environment: environment)
        )
    }
}

@MainActor
final class PlantyAppDelegate: NSObject, UIApplicationDelegate {
    func application(
        _ application: UIApplication,
        didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data
    ) {
        PushRegistrationCenter.shared.didRegister(deviceToken: deviceToken)
    }

    func application(
        _ application: UIApplication,
        didFailToRegisterForRemoteNotificationsWithError error: Error
    ) {
        // Simulators and unsigned local builds can land here. There is nothing
        // to recover in-app; the registration is attempted again next launch.
    }
}
