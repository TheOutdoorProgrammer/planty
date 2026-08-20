import Foundation
import Testing

@testable import Planty

@Suite("Shared interaction state")
struct SharedInteractionStateTests {
    @MainActor
    @Test("Async sheet actions retain failures and clear them on success")
    func asyncSheetFailureLifecycle() async {
        let action = AsyncSheetAction()
        let failure = PlantyError.server(status: 503, message: "not available")

        let failed = await action.perform { failure }
        #expect(!failed)
        #expect(action.error == failure)
        #expect(!action.isRunning)

        let succeeded = await action.perform { nil }
        #expect(succeeded)
        #expect(action.error == nil)
        #expect(!action.isRunning)
    }

    @MainActor
    @Test("Async sheet actions reject a second submit while the first is running")
    func asyncSheetPreventsDoubleSubmit() async {
        let action = AsyncSheetAction()
        let gate = Gate()

        let first = Task { @MainActor in
            await action.perform {
                await gate.wait()
                return nil
            }
        }
        while !action.isRunning { await Task.yield() }

        let second = await action.perform { nil }
        #expect(!second)

        await gate.release()
        #expect(await first.value)
    }

    @MainActor
    @Test("Photo acquisition reports camera failures instead of dropping them")
    func photoAcquisitionShowsFailure() async {
        let acquisition = PhotoAcquisition(capture: { throw CaptureFailure() })

        let photo = await acquisition.takePhoto()

        #expect(photo == nil)
        #expect(acquisition.error == "The camera did not produce a photo. Try again.")
    }

    @MainActor
    @Test("Photo acquisition returns successful camera bytes")
    func photoAcquisitionReturnsBytes() async {
        let jpeg = Data([0xff, 0xd8, 0xff, 0xd9])
        let acquisition = PhotoAcquisition(capture: { jpeg })

        let photo = await acquisition.takePhoto()

        #expect(photo?.jpeg == jpeg)
        #expect(photo?.assetID == nil)
        #expect(acquisition.error == nil)
    }
}

private struct CaptureFailure: Error {}

private actor Gate {
    private var continuation: CheckedContinuation<Void, Never>?

    func wait() async {
        await withCheckedContinuation { continuation = $0 }
    }

    func release() {
        continuation?.resume()
        continuation = nil
    }
}
