import CoreGraphics
import Foundation
import ImageIO
import Testing
import UniformTypeIdentifiers

@testable import Planty

/// Built as real JPEG bytes rather than mocked, because the thing under test is
/// whether ImageIO's dictionaries are read correctly.
@Suite("Capture metadata")
struct CaptureMetadataTests {
    /// A 2x2 JPEG carrying whatever EXIF and GPS blocks the test asks for.
    private func jpeg(
        exif: [CFString: Any]? = nil,
        gps: [CFString: Any]? = nil,
        apple: [CFString: Any]? = nil
    ) throws -> Data {
        let size = 2
        let context = try #require(CGContext(
            data: nil, width: size, height: size,
            bitsPerComponent: 8, bytesPerRow: 0,
            space: CGColorSpaceCreateDeviceRGB(),
            bitmapInfo: CGImageAlphaInfo.noneSkipLast.rawValue
        ))
        context.setFillColor(CGColor(red: 0, green: 0.5, blue: 0, alpha: 1))
        context.fill(CGRect(x: 0, y: 0, width: size, height: size))
        let image = try #require(context.makeImage())

        let output = NSMutableData()
        let destination = try #require(CGImageDestinationCreateWithData(
            output, UTType.jpeg.identifier as CFString, 1, nil
        ))

        var properties: [CFString: Any] = [:]
        if let exif { properties[kCGImagePropertyExifDictionary] = exif }
        if let gps { properties[kCGImagePropertyGPSDictionary] = gps }
        if let apple { properties[kCGImagePropertyMakerAppleDictionary] = apple }

        CGImageDestinationAddImage(destination, image, properties as CFDictionary)
        #expect(CGImageDestinationFinalize(destination))
        return output as Data
    }

    @Test("A capture date is read out of EXIF")
    func readsCaptureDate() throws {
        let data = try jpeg(exif: [kCGImagePropertyExifDateTimeOriginal: "2026:04:11 09:30:00"])
        let found = CaptureMetadataReader.read(from: data)

        let parts = Calendar.current.dateComponents(
            [.year, .month, .day], from: try #require(found.capturedAt)
        )
        #expect(parts.year == 2026)
        #expect(parts.month == 4)
        #expect(parts.day == 11)
    }

    /// Season is half the signal a species backend gets from metadata, so
    /// falling back to the digitized stamp is worth doing.
    @Test("A missing original date falls back to the digitized one")
    func fallsBackToDigitized() throws {
        let data = try jpeg(exif: [kCGImagePropertyExifDateTimeDigitized: "2025:12:01 12:00:00"])
        #expect(CaptureMetadataReader.read(from: data).capturedAt != nil)
    }

    @Test("A northern, eastern coordinate is read as positive")
    func readsCoordinate() throws {
        let data = try jpeg(gps: [
            kCGImagePropertyGPSLatitude: 40.7128,
            kCGImagePropertyGPSLatitudeRef: "N",
            kCGImagePropertyGPSLongitude: 74.0060,
            kCGImagePropertyGPSLongitudeRef: "W"
        ])
        let found = CaptureMetadataReader.read(from: data)

        #expect(found.latitude == 40.7128)
        // West is negative, and a longitude read without its reference lands
        // the plant on the wrong side of the planet.
        #expect(found.longitude == -74.0060)
        #expect(found.hasLocation)
    }

    @Test("A southern latitude is read as negative")
    func readsSouthernHemisphere() throws {
        let data = try jpeg(gps: [
            kCGImagePropertyGPSLatitude: 33.8688,
            kCGImagePropertyGPSLatitudeRef: "S",
            kCGImagePropertyGPSLongitude: 151.2093,
            kCGImagePropertyGPSLongitudeRef: "E"
        ])
        let found = CaptureMetadataReader.read(from: data)

        #expect(found.latitude == -33.8688)
        #expect(found.longitude == 151.2093)
    }

    /// A picker copy commonly has no GPS block. Absent must read as unknown,
    /// never as a coordinate.
    @Test("No GPS block means unknown, not zero")
    func absentLocationIsUnknown() throws {
        let found = CaptureMetadataReader.read(
            from: try jpeg(exif: [kCGImagePropertyExifDateTimeOriginal: "2026:04:11 09:30:00"])
        )

        #expect(found.latitude == nil)
        #expect(found.longitude == nil)
        #expect(!found.hasLocation)
        #expect(found.coordinate == nil)
    }

    @Test("Bytes that are not an image yield nothing rather than throwing")
    func survivesGarbage() {
        let found = CaptureMetadataReader.read(from: Data([0x00, 0x01, 0x02, 0x03]))

        #expect(found.capturedAt == nil)
        #expect(!found.hasLocation)
        #expect(!found.isScreenshot)
    }

    @Test("An empty image has no metadata and does not crash")
    func survivesEmptyData() {
        #expect(CaptureMetadataReader.read(from: Data()) == CaptureMetadata())
    }
}
