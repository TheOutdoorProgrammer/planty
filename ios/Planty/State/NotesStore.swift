import Foundation
import Observation

/// The notes on one plant, and the writing and rewriting of them.
@Observable
@MainActor
final class NotesStore {
    private let api: PlantyAPI

    /// Nil is the household: notes about the place rather than a plant.
    let slug: String?

    private(set) var notes: [PlantNote] = []
    private(set) var isLoading = false
    private(set) var isSaving = false
    var error: PlantyError?

    init(api: PlantyAPI, slug: String? = nil) {
        self.api = api
        self.slug = slug
    }

    func load() async {
        isLoading = true
        defer { isLoading = false }
        do {
            notes = try await load(slug)
            error = nil
        } catch {
            self.error = PlantyError.from(error)
        }
    }

    /// Reports whether it saved, so a sheet knows whether it may close. A sheet
    /// that dismisses on failure loses whatever was typed into it.
    @discardableResult
    func write(title: String, body: String) async -> Bool {
        let text = body.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return false }

        isSaving = true
        defer { isSaving = false }
        do {
            let draft = NoteDraft(title: heading(title), body: text)
            let written = try await write(draft, to: slug)
            notes.insert(written, at: 0)
            error = nil
            return true
        } catch {
            self.error = PlantyError.from(error)
            return false
        }
    }

    @discardableResult
    func rewrite(_ note: PlantNote, title: String, body: String) async -> Bool {
        let text = body.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return false }

        isSaving = true
        defer { isSaving = false }
        do {
            let changed = try await api.updateNote(
                id: note.id,
                draft: NoteDraft(title: heading(title), body: text)
            )
            if let index = notes.firstIndex(where: { $0.id == note.id }) {
                notes[index] = changed
            }
            error = nil
            return true
        } catch {
            self.error = PlantyError.from(error)
            return false
        }
    }

    /// Removed locally only once the server agrees: otherwise a failed delete
    /// leaves a note that looks gone and returns on the next load.
    func remove(_ note: PlantNote) async {
        do {
            try await api.deleteNote(id: note.id)
            notes.removeAll { $0.id == note.id }
            error = nil
        } catch {
            self.error = PlantyError.from(error)
        }
    }

    private func load(_ slug: String?) async throws -> [PlantNote] {
        guard let slug else { return try await api.householdNotes() }
        return try await api.notes(slug: slug)
    }

    private func write(_ draft: NoteDraft, to slug: String?) async throws -> PlantNote {
        guard let slug else { return try await api.addHouseholdNote(draft: draft) }
        return try await api.addNote(slug: slug, draft: draft)
    }

    /// An empty title is no title, not an empty one.
    private func heading(_ title: String) -> String? {
        let trimmed = title.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty ? nil : trimmed
    }
}
