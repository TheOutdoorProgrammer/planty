import Foundation

/// One thing that happened, flattened out of photos, observations and verdicts
/// so the story renders as prose instead of three parallel logs.
struct StoryEvent: Sendable, Hashable, Identifiable {
    let id: UUID
    let occurredAt: Date
    let title: String
    let detail: String?
    let symbol: String
    let photo: Photo?
    let careState: CareState?
}

/// A day in the plant's life. Photos anchor time; the narrative bridges them.
struct StoryChapter: Sendable, Hashable, Identifiable {
    let date: Date
    let title: String
    let narrative: String
    let photo: Photo?
    let events: [StoryEvent]

    var id: Date { date }

    var dateLabel: String {
        date.formatted(.dateTime.month(.wide).day())
    }
}

enum StoryBuilder {
    /// Newest first, one chapter per calendar day.
    static func chapters(
        from timeline: PlantTimeline,
        calendar: Calendar = .current
    ) -> [StoryChapter] {
        let events = flatten(timeline)
        let byDay = Dictionary(grouping: events) { calendar.startOfDay(for: $0.occurredAt) }

        return byDay.keys.sorted(by: >).map { day in
            let dayEvents = (byDay[day] ?? []).sorted { $0.occurredAt > $1.occurredAt }
            return StoryChapter(
                date: day,
                title: title(for: dayEvents),
                narrative: narrative(for: dayEvents),
                photo: dayEvents.compactMap(\.photo).first,
                events: dayEvents
            )
        }
    }

    private static func flatten(_ timeline: PlantTimeline) -> [StoryEvent] {
        let photos = timeline.photos.map { photo in
            StoryEvent(
                id: photo.id,
                occurredAt: photo.takenAt,
                title: photo.isAnalyzed ? "New photo, compared" : "New photo",
                detail: photo.visionFindings ?? photo.caption,
                symbol: "camera.fill",
                photo: photo,
                careState: nil
            )
        }

        let observations = timeline.observations.map { observation in
            StoryEvent(
                id: observation.id,
                occurredAt: observation.occurredAt,
                title: observation.kind.label,
                detail: observation.body,
                symbol: observation.kind.symbol,
                photo: nil,
                careState: nil
            )
        }

        let verdicts = timeline.verdicts.map { verdict in
            StoryEvent(
                id: verdict.id,
                occurredAt: verdict.forDate,
                title: verdict.action.shortLabel,
                detail: verdict.reasoning,
                symbol: CareState.from(action: verdict.action).symbol,
                photo: nil,
                careState: CareState.from(action: verdict.action)
            )
        }

        return photos + observations + verdicts
    }

    private static func title(for events: [StoryEvent]) -> String {
        if let actionable = events.first(where: { $0.careState?.isActionable == true }) {
            return actionable.title
        }
        if events.contains(where: { $0.photo != nil }) {
            return events.first { $0.photo != nil }?.title ?? "New photo"
        }
        return events.first?.title ?? "Recorded"
    }

    private static func narrative(for events: [StoryEvent]) -> String {
        let sentences = events.compactMap(\.detail)
            .map { $0.trimmingCharacters(in: .whitespacesAndNewlines) }
            .filter { !$0.isEmpty }
        guard !sentences.isEmpty else {
            return events.map(\.title).joined(separator: ". ") + "."
        }
        // Two unrelated notes joined by a bare space read as one mangled
        // sentence: a jotting like "gate probe" ran straight into the day's
        // verdict. Anything without an end gets one.
        return sentences.prefix(2).map(ended).joined(separator: " ")
    }

    /// Ends a fragment so the next one cannot run into it.
    private static func ended(_ sentence: String) -> String {
        guard let last = sentence.last, !".!?…".contains(last) else { return sentence }
        return sentence + "."
    }
}
