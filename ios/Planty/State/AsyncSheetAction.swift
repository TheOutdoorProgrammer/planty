import Observation

/// One state machine for a sheet that writes to the service. A write either
/// succeeds, or its error stays visible while the form's local input stays in
/// place. The guard also prevents a double tap from issuing the write twice.
@Observable
@MainActor
final class AsyncSheetAction {
    private(set) var isRunning = false
    private(set) var error: PlantyError?

    /// The primitive operation. Callers with more than one dependent write can
    /// compose them into one Result so the sheet has one busy/error lifecycle.
    @discardableResult
    func performResult<T>(
        _ operation: () async -> Result<T, PlantyError>
    ) async -> Result<T, PlantyError>? {
        guard !isRunning else { return nil }
        isRunning = true
        error = nil
        let result = await operation()
        isRunning = false

        if case .failure(let failure) = result {
            error = failure
        }
        return result
    }

    /// Adapter for the existing store methods whose contract is nil on success
    /// and a PlantyError on failure.
    @discardableResult
    func perform(_ operation: () async -> PlantyError?) async -> Bool {
        let result = await performResult {
            if let failure = await operation() {
                return .failure(failure)
            }
            return .success(())
        }
        if case .success? = result { return true }
        return false
    }

    /// Adapter for direct API calls. The returned value is available only when
    /// the operation succeeded; failures are already retained in `error`.
    func performThrowing<T>(_ operation: () async throws -> T) async -> T? {
        let result = await performResult {
            do {
                return .success(try await operation())
            } catch {
                return .failure(PlantyError.from(error))
            }
        }
        guard case .success(let value)? = result else { return nil }
        return value
    }

    func clearError() { error = nil }
}
