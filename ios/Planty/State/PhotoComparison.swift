import Foundation

/// A plant's photos in order, so two of them can be held against each other.
/// Side by side rather than a wipe: handheld shots weeks apart never line up,
/// so a sliding divider would be comparing backgrounds.
struct PhotoComparison: Sendable, Hashable {
    let ordered: [Photo]

    init(_ photos: [Photo]) {
        ordered = photos.sorted { $0.takenAt < $1.takenAt }
    }

    /// One photo compares to nothing, and offering it invites the reading that
    /// nothing has changed.
    var isPossible: Bool { ordered.count >= 2 }

    var earliest: Photo? { ordered.first }
    var latest: Photo? { ordered.last }

    /// Clamped rather than optional past the ends: a slider bound to a count
    /// that just shrank would otherwise read off the end mid-gesture.
    func photo(at index: Int) -> Photo? {
        guard !ordered.isEmpty else { return nil }
        return ordered[min(max(index, 0), ordered.count - 1)]
    }

    var lastIndex: Int { max(ordered.count - 1, 0) }

    /// "11 weeks apart", which is what makes a pair of photos mean anything.
    /// Same day says so outright, because "0 days apart" reads as a bug.
    static func span(between one: Date, and other: Date, calendar: Calendar = .current) -> String {
        let earlier = min(one, other)
        let later = max(one, other)

        if calendar.isDate(earlier, inSameDayAs: later) {
            return "the same day"
        }

        let formatter = DateComponentsFormatter()
        formatter.calendar = calendar
        formatter.allowedUnits = [.year, .month, .weekOfMonth, .day]
        formatter.maximumUnitCount = 1
        formatter.unitsStyle = .full

        guard let measured = formatter.string(from: earlier, to: later) else {
            return "some time apart"
        }
        return "\(measured) apart"
    }
}
