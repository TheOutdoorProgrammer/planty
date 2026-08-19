import SwiftUI

/// Everything written down about one plant, and the writing of more.
struct NotesScreen: View {
    @State var store: NotesStore
    @State private var editing: PlantNote?
    @State private var writing = false
    @State private var removing: PlantNote?

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 12) {
                if let error = store.error {
                    StateMessage(
                        title: "That did not work.",
                        message: error.errorDescription ?? "Try again in a moment.",
                        accent: PlantyColor.orange,
                        icon: "exclamationmark.triangle.fill"
                    ) {
                        Button("Try again") { Task { await store.load() } }
                            .buttonStyle(SecondaryButtonStyle())
                    }
                }

                if store.notes.isEmpty, !store.isLoading {
                    empty
                }

                ForEach(store.notes) { note in
                    card(note)
                }
            }
            .padding(.horizontal, 20)
            .padding(.vertical, 16)
        }
        .plantyPage()
        .navigationTitle("Notes")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button { writing = true } label: {
                    Label("Write a note", systemImage: "square.and.pencil")
                }
            }
        }
        .task { await store.load() }
        .sheet(isPresented: $writing) {
            NoteSheet(title: "", text: "") { title, body in
                await store.write(title: title, body: body)
            }
        }
        .sheet(item: $editing) { note in
            NoteSheet(title: note.heading ?? "", text: note.body) { title, body in
                await store.rewrite(note, title: title, body: body)
            }
        }
        .confirmationDialog(
            "Delete this note?",
            isPresented: .init(get: { removing != nil }, set: { if !$0 { removing = nil } }),
            titleVisibility: .visible
        ) {
            Button("Delete", role: .destructive) {
                guard let note = removing else { return }
                removing = nil
                Task { await store.remove(note) }
            }
            Button("Keep it", role: .cancel) { removing = nil }
        } message: {
            Text("This cannot be undone.")
        }
    }

    private var empty: some View {
        StateMessage(
            title: "Nothing written down yet.",
            message: """
                Notes are for what the sensors cannot see: where it came \
                from, what it hates, what the cat does to it.
                """,
            accent: PlantyColor.purple,
            icon: "note.text"
        ) {
            Button("Write the first one") { writing = true }
                .buttonStyle(PrimaryButtonStyle())
        }
    }

    private func card(_ note: PlantNote) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            if let heading = note.heading {
                Text(heading)
                    .font(.headline)
                    .foregroundStyle(PlantyColor.foreground)
            }
            Text(note.body)
                .font(.body)
                .foregroundStyle(PlantyColor.foreground)
                .frame(maxWidth: .infinity, alignment: .leading)

            Text(note.wasEdited
                 ? "Edited \(note.updatedAt.formatted(.relative(presentation: .named)))"
                 : note.createdAt.formatted(date: .abbreviated, time: .shortened))
                .font(.caption2)
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .plantyCard(padding: 14)
        .contextMenu {
            Button { editing = note } label: { Label("Edit", systemImage: "pencil") }
            Button(role: .destructive) { removing = note } label: {
                Label("Delete", systemImage: "trash")
            }
        }
    }
}

/// Writes or rewrites one note. Stays open when saving fails, because closing
/// on a failure throws away whatever was typed.
private struct NoteSheet: View {
    @State var title: String
    @State var text: String
    let save: (String, String) async -> Bool

    @Environment(\.dismiss) private var dismiss
    @State private var saving = false
    @FocusState private var bodyFocused: Bool

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("A heading, if it helps", text: $title)
                }
                Section("The note") {
                    TextField("What is worth remembering?", text: $text, axis: .vertical)
                        .lineLimit(4...14)
                        .focused($bodyFocused)
                }
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle(title.isEmpty ? "Note" : title)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") {
                        saving = true
                        Task {
                            let saved = await save(title, text)
                            saving = false
                            if saved { dismiss() }
                        }
                    }
                    .disabled(text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || saving)
                }
            }
            .onAppear { bodyFocused = true }
        }
    }
}
