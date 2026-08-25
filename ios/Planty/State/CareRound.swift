import Foundation

struct CareRoundGroup: Identifiable, Sendable, Equatable {
    let room: String
    let entries: [DigestEntry]
    let reminders: [DueReminder]

    var id: String { room }
    var count: Int { entries.count + reminders.count }
}

enum CareRoundPlanner {
    static func groups(
        digest: Digest?,
        resolvedVerdicts: Set<UUID> = [],
        resolvedReminders: Set<String> = []
    ) -> [CareRoundGroup] {
        guard let digest else { return [] }
        let entries = digest.sortedEntries.filter { !resolvedVerdicts.contains($0.verdict.id) }
        let reminders = digest.sortedDueReminders.filter {
            !resolvedReminders.contains($0.occurrenceID)
        }
        let rooms = Set(entries.map { room(for: $0.plant) } + reminders.map { room(for: $0.plant) })
        return rooms.map { room in
            CareRoundGroup(
                room: room,
                entries: entries.filter { Self.room(for: $0.plant) == room },
                reminders: reminders.filter { Self.room(for: $0.plant) == room }
            )
        }
        .sorted {
            let leftUrgent = $0.entries.contains { $0.verdict.action == .urgent }
            let rightUrgent = $1.entries.contains { $0.verdict.action == .urgent }
            if leftUrgent != rightUrgent { return leftUrgent }
            return $0.room.localizedCaseInsensitiveCompare($1.room) == .orderedAscending
        }
    }

    private static func room(for plant: Plant) -> String {
        let value = (plant.haArea?.nilIfBlank ?? plant.location.nilIfBlank) ?? "Location not recorded"
        return value
    }
}
