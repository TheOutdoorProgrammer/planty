import Observation

/// One state machine for a sheet that writes to the service. A write either
/// succeeds, or its error stays visible while the form's local input stays in
/// place. The guard also prevents a double tap from issuing the write twice.
@Observable
@MainActor
final class AsyncSheetAction {
    private(set) var isRunning = false
    private(set) var error: PlantyError?

    /// Adapter for store methods whose contract is nil on success and a
    /// PlantyError on failure.
    @discardableResult
    func perform(_ operation: () async -> PlantyError?) async -> Bool {
        guard !isRunning else { return false }
        isRunning = true
        error = nil
        defer { isRunning = false }

        if let failure = await operation() {
            error = failure
            return false
        }
        return true
    }

    /// Adapter for direct API calls. The returned value is available only when
    /// the operation succeeded; failures are already retained in `error`.
    func performThrowing<T>(_ operation: () async throws -> T) async -> T? {
        guard !isRunning else { return nil }
        isRunning = true
        error = nil
        defer { isRunning = false }

        do {
            return try await operation()
        } catch {
            self.error = PlantyError.from(error)
            return nil
        }
    }

    func clearError() { error = nil }
}
