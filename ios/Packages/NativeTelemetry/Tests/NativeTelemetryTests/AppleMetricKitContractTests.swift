import Foundation
import Testing
@testable import NativeTelemetry

struct AppleMetricKitContractTests {
    @Test func documentedCallStackTreeMethodOutputProjectsAppFrame() throws {
        let fixture = try #require(Bundle.module.url(
            forResource: "AppleCallStackTree", withExtension: "json", subdirectory: "Fixtures"
        ))
        let data = try Data(contentsOf: fixture)
        let architecture = try #require(NativeArchitecture(rawValue: "arm64"))
        let crash = try #require(MetricKitProjection.crash(
            stackJSON: data, executableName: "MetricKitTestApp", architecture: architecture
        ))
        #expect(crash.imageUUID.uuidString == "70B89F27-1634-3580-A695-57CDB41D7743")
        #expect(crash.frames.map(\.offset) == [165304])
        #expect(crash.architecture == .arm64)
        let projected = try #require(String(data: NativeCoding.encoder().encode(crash), encoding: .utf8))
        #expect(!projected.contains("7170766264"))
        #expect(!projected.contains("libdyld"))
    }
}
