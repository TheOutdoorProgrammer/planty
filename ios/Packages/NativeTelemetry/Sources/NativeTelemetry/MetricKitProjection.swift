import Foundation

public enum MetricKitProjection {
    public static func crash(
        stackJSON: Data, executableName: String, architecture: NativeArchitecture
    ) -> NativeCrash? {
        guard stackJSON.count <= 4_194_304,
              let object = try? JSONSerialization.jsonObject(with: stackJSON) as? [String: Any],
              let tree = object["callStackTree"] as? [String: Any],
              let stacks = tree["callStacks"] as? [[String: Any]] else { return nil }
        let selected = stacks.first { $0["threadAttributed"] as? Bool == true } ?? stacks.first
        var pending = Array(((selected?["callStackRootFrames"] as? [[String: Any]]) ?? []).reversed())
        var frames: [NativeCrash.Frame] = []
        var imageUUID: UUID?
        var inspected = 0
        while let frame = pending.popLast(), inspected < 4_096, frames.count < 64 {
            inspected += 1
            if frame["binaryName"] as? String == executableName,
               let rawUUID = frame["binaryUUID"] as? String,
               let uuid = UUID(uuidString: rawUUID),
               let number = frame["offsetIntoBinaryTextSegment"] as? NSNumber,
               String(cString: number.objCType) != "c",
               let offset = UInt64(number.stringValue), offset < 1 << 40,
               imageUUID == nil || imageUUID == uuid {
                imageUUID = uuid
                frames.append(.init(offset: offset))
            }
            if let children = frame["subFrames"] as? [[String: Any]] {
                pending.append(contentsOf: children.prefix(4_096 - inspected).reversed())
            }
        }
        guard let imageUUID else { return nil }
        return NativeCrash(imageUUID: imageUUID, architecture: architecture, frames: frames)
    }
}

#if canImport(MetricKit) && os(iOS)
import MetricKit

public final class MetricKitCapture: NSObject, MXMetricManagerSubscriber, @unchecked Sendable {
    private let executableName: String
    private let receive: @Sendable (NativeRelease, NativeEvent) -> Void

    public init(executableName: String, receive: @escaping @Sendable (NativeRelease, NativeEvent) -> Void) {
        self.executableName = executableName
        self.receive = receive
        super.init()
    }

    public func start() { MXMetricManager.shared.add(self) }

    deinit { MXMetricManager.shared.remove(self) }

    public func didReceive(_ payloads: [MXDiagnosticPayload]) {
        for payload in payloads.prefix(16) {
            for diagnostic in (payload.crashDiagnostics ?? []).prefix(16) {
                capture(diagnostic, tree: diagnostic.callStackTree, timestamp: payload.timeStampEnd, hang: false)
            }
            for diagnostic in (payload.hangDiagnostics ?? []).prefix(16) {
                capture(diagnostic, tree: diagnostic.callStackTree, timestamp: payload.timeStampEnd, hang: true)
            }
        }
    }

    private func capture(_ diagnostic: MXDiagnostic, tree: MXCallStackTree, timestamp: Date, hang: Bool) {
        guard let release = NativeRelease(
            release: diagnostic.applicationVersion, build: diagnostic.metaData.applicationBuildVersion
        ), let architecture = NativeArchitecture(rawValue: diagnostic.metaData.platformArchitecture),
        let crash = MetricKitProjection.crash(
            stackJSON: tree.jsonRepresentation(), executableName: executableName, architecture: architecture
        ) else { return }
        receive(release, NativeEvent(
            timestamp: timestamp, operation: hang ? .appHang : .appCrash,
            outcome: .failure, errorClass: hang ? .hang : .crash, crash: crash
        ))
    }
}
#endif
