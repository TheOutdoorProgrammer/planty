import Foundation
import Testing

@testable import Planty

private struct StubUpdates: FledgeUpdating {
    var release: FledgeRelease?

    func check(runningBuild: String) async -> FledgeRelease? { release }
}

private func release(build: String, available: Bool, notes: String? = nil) -> FledgeRelease {
    FledgeRelease(
        version: "1.0",
        build: build,
        installPageURL: URL(string: "https://fledge.test/a/zone.stout.Planty")!,
        updateAvailable: available,
        notes: notes.map { [FledgeNote(version: "1.0", build: build, notes: $0)] } ?? []
    )
}

@Suite("Update check")
struct UpdateCheckTests {
    @Test("A newer build is offered")
    @MainActor
    func offersNewerBuild() async {
        let store = UpdateStore(
            service: StubUpdates(release: release(build: "5", available: true)),
            runningBuild: "4"
        )
        await store.check()

        #expect(store.available?.build == "5")
        #expect(store.available?.label == "1.0 (5)")
    }

    /// Fledge answers on every check, whether or not there is anything new, so
    /// the flag is what decides rather than the presence of a reply.
    @Test("Being current shows nothing")
    func currentShowsNothing() async {
        let store = await UpdateStore(
            service: StubUpdates(release: release(build: "4", available: false)),
            runningBuild: "4"
        )
        await store.check()

        #expect(await store.available == nil)
    }

    /// Distribution is not the app's job. A phone off the network, or a server
    /// too old to know the route, must not produce an error state.
    @Test("An unreachable server is silence, not an error")
    func unreachableIsSilent() async {
        let store = await UpdateStore(service: StubUpdates(release: nil), runningBuild: "4")
        await store.check()

        #expect(await store.available == nil)
    }

    /// A simulator build has no CFBundleVersion worth sending, and asking with
    /// an empty build makes Fledge answer with the whole changelog.
    @Test("No running build means no check")
    func skipsWithoutABuildNumber() async {
        let store = await UpdateStore(
            service: StubUpdates(release: release(build: "9", available: true)),
            runningBuild: ""
        )
        await store.check()

        #expect(await store.available == nil)
    }

    @Test("Dismissing puts the banner away")
    @MainActor
    func dismissClears() async {
        let store = UpdateStore(
            service: StubUpdates(release: release(build: "5", available: true)),
            runningBuild: "4"
        )
        await store.check()
        store.dismiss()

        #expect(store.available == nil)
    }

    /// Decoded from the shape the running Fledge server documents, so a change
    /// to its field names fails here rather than silently never updating.
    @Test("Fledge's own response shape decodes")
    func decodesFledgeResponse() throws {
        let json = """
            {
              "bundle_id": "zone.stout.Planty",
              "name": "Planty",
              "version": "1.0",
              "build": "5",
              "build_id": "abc123",
              "published": "2026-08-18T20:00:00Z",
              "size": 1234,
              "install_page_url": "https://fledge.test/a/zone.stout.Planty",
              "update_available": true,
              "expired": false,
              "changelog": [
                {"version": "1.0", "build": "5", "build_id": "abc123",
                 "published": "2026-08-18T20:00:00Z", "notes": "Fixes refresh."}
              ]
            }
            """
        let decoded = try JSONDecoder().decode(FledgeRelease.self, from: Data(json.utf8))

        #expect(decoded.updateAvailable)
        #expect(decoded.label == "1.0 (5)")
        #expect(decoded.notes.first?.notes == "Fixes refresh.")
    }
}
