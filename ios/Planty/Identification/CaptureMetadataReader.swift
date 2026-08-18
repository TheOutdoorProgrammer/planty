import Foundation
import ImageIO

/// Reads what the original file says about itself.
///
/// Deliberately free of Photos and Vision so it can be tested against bytes.
enum CaptureMetadataReader {
    /// Everything is optional because a picker copy usually has GPS stripped,
    /// and reading that as "no location" rather than "unknown" is how a
    /// houseplant ends up geolocated at the equator.
    static func read(from data: Data) -> CaptureMetadata {
        var found = CaptureMetadata()

        guard
            let source = CGImageSourceCreateWithData(data as CFData, nil),
            let properties = CGImageSourceCopyPropertiesAtIndex(source, 0, nil)
                as? [CFString: Any]
        else {
            return found
        }

        if let exif = properties[kCGImagePropertyExifDictionary] as? [CFString: Any] {
            found.capturedAt = captureDate(from: exif)
        }
        if let gps = properties[kCGImagePropertyGPSDictionary] as? [CFString: Any] {
            let position = coordinate(from: gps)
            found.latitude = position?.latitude
            found.longitude = position?.longitude
            found.altitude = altitude(from: gps)
        }
        if let apple = properties[kCGImagePropertyMakerAppleDictionary] as? [CFString: Any] {
            found.isScreenshot = isScreenshot(apple)
        }

        return found
    }

    /// EXIF dates carry no zone, so they are read in the device's current one.
    /// Wrong by hours at worst, which does not move a season.
    private static func captureDate(from exif: [CFString: Any]) -> Date? {
        let raw = exif[kCGImagePropertyExifDateTimeOriginal] as? String
            ?? exif[kCGImagePropertyExifDateTimeDigitized] as? String
        guard let raw else { return nil }

        let formatter = DateFormatter()
        formatter.dateFormat = "yyyy:MM:dd HH:mm:ss"
        formatter.timeZone = .current
        formatter.locale = Locale(identifier: "en_US_POSIX")
        return formatter.date(from: raw)
    }

    /// GPS stores magnitude and hemisphere separately, so a southern latitude
    /// read without its reference lands in the wrong hemisphere.
    private static func coordinate(from gps: [CFString: Any]) -> (latitude: Double, longitude: Double)? {
        guard
            let latitude = gps[kCGImagePropertyGPSLatitude] as? Double,
            let longitude = gps[kCGImagePropertyGPSLongitude] as? Double
        else { return nil }

        let latitudeRef = gps[kCGImagePropertyGPSLatitudeRef] as? String ?? "N"
        let longitudeRef = gps[kCGImagePropertyGPSLongitudeRef] as? String ?? "E"

        return (
            latitude: latitudeRef.uppercased() == "S" ? -latitude : latitude,
            longitude: longitudeRef.uppercased() == "W" ? -longitude : longitude
        )
    }

    private static func altitude(from gps: [CFString: Any]) -> Double? {
        guard let altitude = gps[kCGImagePropertyGPSAltitude] as? Double else { return nil }
        // Reference 1 means below sea level.
        let below = (gps[kCGImagePropertyGPSAltitudeRef] as? Int) == 1
        return below ? -altitude : altitude
    }

    /// Apple does not publish these keys. 0x0014 has carried an image-source
    /// code for several releases, where 5 is a screenshot, so this is a hint
    /// treated as advisory rather than a fact worth blocking on.
    private static func isScreenshot(_ apple: [CFString: Any]) -> Bool {
        (apple["14" as CFString] as? Int) == 5
    }
}
