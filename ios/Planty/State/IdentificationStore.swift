import Foundation
import Observation

@Observable
@MainActor
final class IdentificationStore {
    enum Stage: Sendable, Equatable {
        case idle
        case working
        case done(IdentificationOutcome)

        var isWorking: Bool { self == .working }
    }

    private(set) var stage = Stage.idle

    private let pipeline: IdentificationPipeline

    init(pipeline: IdentificationPipeline) {
        self.pipeline = pipeline
    }

    /// Vision and the network both run off the main actor: the pipeline is a
    /// plain Sendable struct, so awaiting it never parks the UI.
    func identify(jpeg: Data, assetID: String?) async {
        stage = .working
        let outcome = await pipeline.identify(pickedData: jpeg, assetID: assetID)
        stage = .done(outcome)
    }

    func reset() { stage = .idle }

    var outcome: IdentificationOutcome? {
        if case .done(let outcome) = stage { return outcome }
        return nil
    }
}
