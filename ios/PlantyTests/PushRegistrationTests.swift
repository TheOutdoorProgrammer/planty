import Foundation
import Testing

@testable import Planty

@MainActor
@Suite("Push registration health")
struct PushRegistrationTests {
    private func defaults() -> UserDefaults {
        let name = "PushRegistrationTests.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: name)!
        defaults.removePersistentDomain(forName: name)
        return defaults
    }

    @Test("A refreshed APNs token replaces the same installation identity")
    func tokenRefresh() async {
        let api = FakeAPI()
        let center = PushRegistrationCenter(defaults: defaults(), environment: "production")
        center.configure(api: api, serviceID: "https://planty-a.test")

        center.didRegister(deviceToken: Data([0xAA]))
        await center.synchronize()
        center.didRegister(deviceToken: Data([0xBB]))
        await center.synchronize()

        let registrations = api.pushRegistrations
        #expect(registrations.last?.token == "bb")
        #expect(Set(registrations.map(\.installationID)).count == 1)
        #expect(center.tokenUpload == .accepted(.reference))
    }

    @Test("Changing the service uploads the current token to the new service")
    func serviceChange() async {
        let first = FakeAPI()
        let second = FakeAPI()
        let center = PushRegistrationCenter(defaults: defaults(), environment: "production")
        center.configure(api: first, serviceID: "https://planty-a.test")
        center.didRegister(deviceToken: Data([0xAB, 0xCD]))
        await center.synchronize()

        center.configure(api: second, serviceID: "https://planty-b.test")
        await center.synchronize()

        #expect(first.pushRegistrations.last?.token == "abcd")
        #expect(second.pushRegistrations.last?.token == "abcd")
        #expect(first.pushRegistrations.last?.installationID == second.pushRegistrations.last?.installationID)
    }

    @Test("A reinstall gets a new identity and an environment change is explicit")
    func reinstallAndEnvironmentChange() async {
        let old = PushRegistrationCenter(defaults: defaults(), environment: "sandbox")
        let fresh = PushRegistrationCenter(defaults: defaults(), environment: "production")

        #expect(old.installationID != fresh.installationID)
        #expect(old.environment == "sandbox")
        #expect(fresh.environment == "production")
    }

    @Test("A real test action reaches the APNs test endpoint seam")
    func testNotification() async {
        let api = FakeAPI()
        let center = PushRegistrationCenter(defaults: defaults(), environment: "production")
        center.configure(api: api, serviceID: "https://planty.test")

        await center.testNotification()

        #expect(api.pushTests.last?.installationID == center.installationID)
        guard case .accepted = center.testDelivery else {
            Issue.record("test delivery did not report APNs acceptance")
            return
        }
    }

    @Test("Push destinations preserve plant and settings routes")
    func routes() {
        let plant = PlantyPushRoute(userInfo: [
            "destination": ["kind": "plant", "plant_slug": "mona"]
        ])
        let settings = PlantyPushRoute(userInfo: [
            "destination": ["kind": "settings"]
        ])

        guard case .plant(let slug) = plant else {
            Issue.record("plant destination did not parse")
            return
        }
        #expect(slug == "mona")
        guard case .settings = settings else {
            Issue.record("settings destination did not parse")
            return
        }
    }

    @Test("A push destination is not hidden behind an open settings sheet")
    func routedPushDismissesSettings() {
        let session = AppSession(
            defaults: defaults(),
            tokens: InMemoryTokenStore(),
            api: FakeAPI()
        )
        session.isShowingSettings = true

        session.openPushRoute(.plant("mona"))

        #expect(!session.isShowingSettings)
        #expect(session.selectedTab == .plants)
        #expect(session.pendingPlantSlug == "mona")

        session.openPushRoute(.settings)
        #expect(session.isShowingSettings)
    }
}
