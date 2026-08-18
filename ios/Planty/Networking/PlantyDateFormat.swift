import Foundation

/// Go writes RFC3339 with up to nine fractional digits, and none at all when
/// zero, so no single ISO8601DateFormatter reads every timestamp it emits.
/// The fraction is clipped to three digits because the parser rejects more.
enum PlantyDateFormat {
    // ISO8601DateFormatter is documented thread-safe for parsing, and building
    // one per timestamp shows up immediately on a long timeline.
    nonisolated(unsafe) private static let fractional: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
        return formatter
    }()

    nonisolated(unsafe) private static let whole: ISO8601DateFormatter = {
        let formatter = ISO8601DateFormatter()
        formatter.formatOptions = [.withInternetDateTime]
        return formatter
    }()

    static func date(from raw: String) -> Date? {
        let clipped = clippingFraction(raw)
        return fractional.date(from: clipped) ?? whole.date(from: clipped)
    }

    static func string(from date: Date) -> String {
        fractional.string(from: date)
    }

    /// "2026-08-18T08:04:32.123456789Z" becomes ".123Z"; anything without a
    /// fraction, or already short enough, is returned untouched.
    static func clippingFraction(_ raw: String) -> String {
        guard let dot = raw.firstIndex(of: ".") else { return raw }

        var digitsEnd = raw.index(after: dot)
        while digitsEnd < raw.endIndex, raw[digitsEnd].isNumber {
            digitsEnd = raw.index(after: digitsEnd)
        }

        let digits = raw.distance(from: raw.index(after: dot), to: digitsEnd)
        guard digits > 3 else { return raw }

        let keep = raw.index(dot, offsetBy: 4)
        return String(raw[raw.startIndex..<keep]) + String(raw[digitsEnd...])
    }
}

extension JSONDecoder.DateDecodingStrategy {
    static let planty = Self.custom { decoder in
        let raw = try decoder.singleValueContainer().decode(String.self)
        guard let date = PlantyDateFormat.date(from: raw) else {
            throw DecodingError.dataCorrupted(
                DecodingError.Context(
                    codingPath: decoder.codingPath,
                    debugDescription: "Not an RFC3339 timestamp: \(raw)"
                )
            )
        }
        return date
    }
}

extension JSONEncoder.DateEncodingStrategy {
    static let planty = Self.custom { date, encoder in
        var container = encoder.singleValueContainer()
        try container.encode(PlantyDateFormat.string(from: date))
    }
}

enum PlantyCoders {
    static func decoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        decoder.dateDecodingStrategy = .planty
        return decoder
    }

    static func encoder() -> JSONEncoder {
        let encoder = JSONEncoder()
        encoder.dateEncodingStrategy = .planty
        return encoder
    }
}
