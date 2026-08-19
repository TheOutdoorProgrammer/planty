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
    private var activeRequest: UUID?

    init(pipeline: IdentificationPipeline) {
        self.pipeline = pipeline
    }

    /// Vision and the network both run off the main actor: the pipeline is a
    /// plain Sendable struct, so awaiting it never parks the UI.
    func identify(jpeg: Data, assetID: String?) async {
        let request = UUID()
        activeRequest = request
        stage = .working
        let outcome = await pipeline.identify(pickedData: jpeg, assetID: assetID)
        guard activeRequest == request else { return }
        stage = .done(outcome)
    }

    func reset() {
        activeRequest = nil
        stage = .idle
    }

    var outcome: IdentificationOutcome? {
        if case .done(let outcome) = stage { return outcome }
        return nil
    }
}
