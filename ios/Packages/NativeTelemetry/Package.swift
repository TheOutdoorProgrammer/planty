// swift-tools-version: 6.0
import PackageDescription

let package = Package(
    name: "NativeTelemetry",
    platforms: [.iOS(.v17), .macOS(.v13)],
    products: [.library(name: "NativeTelemetry", targets: ["NativeTelemetry"])],
    targets: [
        .target(name: "NativeTelemetry"),
        .testTarget(name: "NativeTelemetryTests", dependencies: ["NativeTelemetry"])
    ]
)
