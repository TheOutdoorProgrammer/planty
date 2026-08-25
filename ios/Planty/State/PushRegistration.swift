import Observation
import UIKit
import UserNotifications

enum PushProgress: Equatable {
    case idle
    case pending
    case accepted(Date)
    case failed(String)
}

enum PlantyPushRoute: Sendable {
    case today
    case settings
    case capture
    case plant(String?)

    init(userInfo: [AnyHashable: Any]) {
        let destination = userInfo["destination"] as? [String: Any]
        let data = userInfo["data"] as? [String: Any]
        let kind = destination?["kind"] as? String ?? userInfo["screen"] as? String
        let slug = destination?["plant_slug"] as? String
            ?? userInfo["plant_slug"] as? String
            ?? data?["plant_slug"] as? String
        switch kind {
        case "settings": self = .settings
        case "plant": self = .plant(slug)
        case "capture": self = .capture
        default: self = .today
        }
    }
}

/// The five independent links in notification delivery.
/// A healthy HTTP service is deliberately not one of them.
@Observable
@MainActor
final class PushRegistrationCenter {
    static let shared = PushRegistrationCenter()

    private(set) var permission: UNAuthorizationStatus = .notDetermined
    private(set) var apnsRegistration: PushProgress = .idle
    private(set) var tokenUpload: PushProgress = .idle
    private(set) var testDelivery: PushProgress = .idle
    private(set) var serverStatus: PushServerStatus?
    private(set) var lastRegistrationError: String?

    let installationID: UUID
    let environment: String

    private var api: (any PlantyAPI)?
    private var token: String?
    private var serviceID = ""
    private let defaults: UserDefaults

    init(
        defaults: UserDefaults = .standard,
        environment: String = PushRegistrationCenter.buildEnvironment
    ) {
        self.defaults = defaults
        self.environment = environment
        if let raw = defaults.string(forKey: "planty.push.installation_id"),
           let saved = UUID(uuidString: raw) {
            installationID = saved
        } else {
            let created = UUID()
            installationID = created
            defaults.set(created.uuidString, forKey: "planty.push.installation_id")
        }
        lastRegistrationError = defaults.string(forKey: "planty.push.apns_error")
    }

    func configure(api: any PlantyAPI, serviceID: String) {
        self.api = api
        let changed = self.serviceID != serviceID
        self.serviceID = serviceID
        if changed {
            tokenUpload = .idle
            serverStatus = nil
            testDelivery = .idle
        }
        Task { await synchronize() }
    }

    func requestAuthorization() async {
        await refreshPermission()
        do {
            if permission == .notDetermined {
                _ = try await UNUserNotificationCenter.current().requestAuthorization(
                    options: [.alert, .badge, .sound]
                )
                await refreshPermission()
            }
            guard permission == .authorized || permission == .provisional || permission == .ephemeral else {
                return
            }
            apnsRegistration = .pending
            UIApplication.shared.registerForRemoteNotifications()
        } catch {
            rememberRegistrationError(error.localizedDescription)
        }
    }

    func refreshPermission() async {
        permission = await UNUserNotificationCenter.current().notificationSettings().authorizationStatus
    }

    func didRegister(deviceToken: Data) {
        token = deviceToken.map { String(format: "%02x", $0) }.joined()
        apnsRegistration = .accepted(Date())
        rememberRegistrationError(nil)
        Task { await synchronize() }
    }

    func didFail(error: Error) {
        apnsRegistration = .failed(error.localizedDescription)
        rememberRegistrationError(error.localizedDescription)
    }

    func synchronize() async {
        await syncToken()
        await refreshServerHealth()
    }

    func testNotification() async {
        guard let api else {
            testDelivery = .failed("The Planty service is not configured.")
            return
        }
        testDelivery = .pending
        do {
            try await api.testPush(
                PushInstallationRequest(
                    installationID: installationID,
                    environment: environment
                )
            )
            testDelivery = .accepted(Date())
        } catch {
            testDelivery = .failed(
                PlantyError.from(error).errorDescription ?? error.localizedDescription
            )
        }
    }

    func recover() async {
        await refreshPermission()
        if permission == .denied {
            guard let url = URL(string: UIApplication.openSettingsURLString) else { return }
            await UIApplication.shared.open(url)
            return
        }
        if token != nil {
            await synchronize()
        } else {
            apnsRegistration = .pending
            UIApplication.shared.registerForRemoteNotifications()
        }
    }

    private func syncToken() async {
        guard let token, let api else { return }
        tokenUpload = .pending
        do {
            let receipt = try await api.registerPushDevice(
                PushDeviceRegistration(
                    token: token,
                    environment: environment,
                    installationID: installationID
                )
            )
            tokenUpload = .accepted(receipt.acceptedAt)
            defaults.set(receipt.acceptedAt, forKey: acceptanceKey)
        } catch {
            tokenUpload = .failed(
                PlantyError.from(error).errorDescription ?? error.localizedDescription
            )
        }
    }

    private func refreshServerHealth() async {
        guard let api else { return }
        do {
            let health = try await api.pushHealth(
                installationID: installationID,
                environment: environment
            )
            serverStatus = health.server
            if let receipt = health.registration {
                tokenUpload = .accepted(receipt.acceptedAt)
                defaults.set(receipt.acceptedAt, forKey: acceptanceKey)
            } else if token == nil, let accepted = defaults.object(forKey: acceptanceKey) as? Date {
                tokenUpload = .accepted(accepted)
            }
        } catch {
            if case .accepted = tokenUpload { return }
            tokenUpload = .failed(
                PlantyError.from(error).errorDescription ?? error.localizedDescription
            )
        }
    }

    private func rememberRegistrationError(_ message: String?) {
        lastRegistrationError = message
        defaults.set(message, forKey: "planty.push.apns_error")
    }

    private var acceptanceKey: String {
        "planty.push.accepted.\(serviceID).\(environment)"
    }

    private static var buildEnvironment: String {
        #if DEBUG
        "sandbox"
        #else
        "production"
        #endif
    }
}

@MainActor
final class PlantyAppDelegate: NSObject, UIApplicationDelegate {
    func application(
        _ application: UIApplication,
        didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]? = nil
    ) -> Bool {
        UNUserNotificationCenter.current().delegate = self
        return true
    }

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
        PushRegistrationCenter.shared.didFail(error: error)
    }
}

extension PlantyAppDelegate: UNUserNotificationCenterDelegate {
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse
    ) async {
        let route = PlantyPushRoute(userInfo: response.notification.request.content.userInfo)
        await MainActor.run {
            NotificationCenter.default.post(name: .plantyPushOpened, object: route)
        }
    }
}

extension Notification.Name {
    static let plantyPushOpened = Notification.Name("planty.push.opened")
}
