import Foundation

extension AppSession {
    func startPushNotifications() async {
        // A hosted unit-test bundle launches enough of the SwiftUI app for this
        // task to run. Never put a system permission sheet in front of XCTest.
        guard ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] == nil else {
            return
        }
        PushRegistrationCenter.shared.configure(api: api)
        await PushRegistrationCenter.shared.requestAuthorization()
    }
}
