import Foundation
import Observation

@Observable
@MainActor
final class UpdateStore {
    private(set) var available: FledgeRelease?

    private let service: any FledgeUpdating
    private let runningBuild: String

    init(service: any FledgeUpdating, runningBuild: String = FledgeUpdateService.runningBuild) {
        self.service = service
        self.runningBuild = runningBuild
    }

    /// Checked once on launch. Nothing about a plant depends on this, so it
    /// never blocks, never retries and never surfaces a failure.
    func check() async {
        guard !runningBuild.isEmpty else { return }
        let latest = await service.check(runningBuild: runningBuild)
        available = (latest?.updateAvailable == true) ? latest : nil
    }

    func dismiss() { available = nil }
}
