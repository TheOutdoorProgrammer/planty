import Foundation
import NativeTelemetry
import OSLog

@MainActor
final class PlantyTelemetry {
    static let shared = PlantyTelemetry()

    private var queue: NativeTelemetryQueue?
    private var transport: NativeHTTPTransport?
    private var capture: MetricKitCapture?
    private var deliveryTask: Task<Void, Never>?
    private var release: NativeRelease?
    private var started = false
    private var deliveryPaused = false
    private var deliveryGeneration = 0
    private let logger = Logger(subsystem: "zone.stout.Planty", category: "NativeTelemetry")

    func start() {
        guard !started, ProcessInfo.processInfo.environment["XCTestConfigurationFilePath"] == nil else { return }
        started = true
        release = NativeRelease(
            release: Bundle.main.object(forInfoDictionaryKey: "CFBundleShortVersionString") as? String ?? "",
            build: Bundle.main.object(forInfoDictionaryKey: "CFBundleVersion") as? String ?? ""
        )
        do {
            let directory = try FileManager.default.url(
                for: .applicationSupportDirectory, in: .userDomainMask, appropriateFor: nil, create: true
            ).appendingPathComponent("NativeTelemetry", isDirectory: true)
            queue = try NativeTelemetryQueue(directory: directory)
        } catch {
            logger.error("Native telemetry queue unavailable")
            return
        }
        let executable = Bundle.main.object(forInfoDictionaryKey: "CFBundleExecutable") as? String ?? "Planty"
        capture = MetricKitCapture(executableName: executable) { release, event in
            Task { @MainActor in await PlantyTelemetry.shared.accept(event, release: release) }
        }
        capture?.start()
        Task { await record(operation: .appStart) }
    }

    func configure(_ configuration: PlantyConfiguration) {
        deliveryGeneration += 1
        deliveryTask?.cancel()
        deliveryTask = nil
        if let baseURL = configuration.baseURL, let token = configuration.token {
            transport = NativeHTTPTransport(baseURL: baseURL, bearer: token)
        } else {
            transport = nil
        }
        scheduleDelivery()
    }

    func record(
        operation: NativeOperation, outcome: NativeOutcome = .success,
        durationMS: Int = 0, errorClass: NativeErrorClass = .none
    ) async {
        guard let release else { return }
        await accept(NativeEvent(
            operation: operation, outcome: outcome, durationMS: durationMS, errorClass: errorClass
        ), release: release)
    }

    func pauseDelivery() {
        deliveryPaused = true
        deliveryGeneration += 1
        deliveryTask?.cancel()
        deliveryTask = nil
    }

    func resumeDelivery() {
        deliveryPaused = false
        scheduleDelivery()
    }

    func record(error: PlantyError, durationMS: Int = 0) async {
        let classification: NativeErrorClass
        switch error {
        case .unauthorized: classification = .unauthorized
        case .offline, .transport: classification = .transport
        case .timedOut: classification = .timeout
        case .server: classification = .server
        case .decoding: classification = .decoding
        case .cancelled: classification = .none
        default: classification = .other
        }
        await record(
            operation: .apiRequest, outcome: error == .cancelled ? .cancelled : .failure,
            durationMS: durationMS, errorClass: classification
        )
    }

    private func accept(_ event: NativeEvent, release: NativeRelease) async {
        guard let queue else { return }
        do {
            try await queue.enqueue(event, release: release)
            scheduleDelivery()
        } catch {
            logger.error("Native telemetry event could not be persisted")
            return
        }
    }

    private func scheduleDelivery() {
        guard !deliveryPaused, deliveryTask == nil, let queue, let transport else { return }
        let generation = deliveryGeneration
        deliveryTask = Task {
            defer { if generation == deliveryGeneration { deliveryTask = nil } }
            while !Task.isCancelled {
                do {
                    try await Task.sleep(for: .seconds(1))
                    try await queue.drain(using: transport)
                    if await queue.counts().pending == 0 { return }
                    try await Task.sleep(for: .seconds(60))
                } catch {
                    return
                }
            }
        }
    }
}
