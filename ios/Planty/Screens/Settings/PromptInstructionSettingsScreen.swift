import SwiftUI

struct PromptInstructionSettingsScreen: View {
    @Environment(AppSession.self) private var session

    private var jobs: [AIJob] { AIJob.allCases.filter { $0 != .unknown } }

    var body: some View {
        List {
            Section {
                Label("Editable overlay only", systemImage: "text.badge.plus")
                    .font(.headline)
                    .foregroundStyle(PlantyColor.purple)
                Text("Each overlay is appended to Planty's code-owned prompt. It cannot replace the base prompt, safety rules, response schema, tool policy, or authority boundaries.")
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            Section("All six jobs") {
                if let error = session.promptInstructions.error {
                    Text(error.errorDescription ?? "Prompt overlays could not be loaded.")
                        .foregroundStyle(PlantyColor.orange)
                }
                ForEach(jobs, id: \.self) { job in
                    NavigationLink {
                        PromptInstructionEditor(job: job)
                    } label: {
                        PromptInstructionRow(
                            job: job,
                            instruction: session.promptInstructions.instruction(for: job)
                        )
                    }
                }
            }
        }
        .scrollContentBackground(.hidden)
        .plantyPage()
        .navigationTitle("Prompt overlays")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable { await session.promptInstructions.load() }
        .task { await session.promptInstructions.loadIfNeeded() }
    }
}

private struct PromptInstructionRow: View {
    let job: AIJob
    let instruction: PromptInstructionSetting

    var body: some View {
        VStack(alignment: .leading, spacing: 5) {
            Text(job.label).font(.body.weight(.semibold))
            Text(instruction.hasOverlay ? "Custom overlay active" : "Code-owned base prompt only")
                .font(.caption)
                .foregroundStyle(instruction.hasOverlay ? PlantyColor.purple : PlantyColor.secondaryText)
            if let updatedAt = instruction.updatedAt {
                Text("Updated \(updatedAt.formatted(date: .abbreviated, time: .shortened))")
                    .font(.caption2)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
        .accessibilityElement(children: .combine)
    }
}

private struct PromptInstructionEditor: View {
    let job: AIJob

    @Environment(AppSession.self) private var session
    @State private var text = ""
    @State private var loadedText = ""
    @State private var failure: PlantyError?
    @State private var confirmsReset = false

    private var draft: PromptOverlayDraft { PromptOverlayDraft(instructions: text) }
    private var instruction: PromptInstructionSetting { session.promptInstructions.instruction(for: job) }

    var body: some View {
        Form {
            Section {
                Text("This text is an overlay appended to the base prompt. The base prompt stays in code and remains authoritative for safety, schema, tools, and permissions.")
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            } header: {
                Text("Editable overlay")
            }

            if let failure {
                Section {
                    SheetErrorRow(headline: "The overlay was not changed.", error: failure)
                }
            }

            Section {
                TextEditor(text: $text)
                    .frame(minHeight: 220)
                    .font(.body.monospaced())
                    .accessibilityLabel("Instruction overlay for \(job.label)")
            } header: {
                Text(job.label)
            } footer: {
                Text("\(text.lengthOfBytes(using: .utf8).formatted()) of 12,000 bytes. This does not replace the base prompt.")
            }

            Section {
                Button("Save overlay") { Task { await save() } }
                    .disabled(!draft.isValid || text == loadedText || session.promptInstructions.saving.contains(job))
                if instruction.hasOverlay {
                    Button("Reset to code-owned prompt", role: .destructive) { confirmsReset = true }
                        .disabled(session.promptInstructions.saving.contains(job))
                }
            }
        }
        .scrollContentBackground(.hidden)
        .plantyPage()
        .navigationTitle(job.label)
        .navigationBarTitleDisplayMode(.inline)
        .onAppear { synchronize() }
        .onChange(of: instruction.instructions) { _, _ in synchronize() }
        .confirmationDialog(
            "Remove this editable overlay?",
            isPresented: $confirmsReset,
            titleVisibility: .visible
        ) {
            Button("Reset to code-owned prompt", role: .destructive) { Task { await reset() } }
            Button("Keep overlay", role: .cancel) {}
        } message: {
            Text("The immutable base prompt remains. Only the additional editable instructions are removed.")
        }
    }

    private func synchronize() {
        text = instruction.instructions
        loadedText = instruction.instructions
    }

    private func save() async {
        failure = await session.promptInstructions.save(draft, for: job)
        if failure == nil { synchronize() }
    }

    private func reset() async {
        failure = await session.promptInstructions.reset(job)
        if failure == nil { synchronize() }
    }
}
