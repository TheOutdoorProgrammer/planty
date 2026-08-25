import SwiftUI

struct PlantHealthSection: View {
    let plant: Plant

    @Environment(AppSession.self) private var session
    @State private var historyExpanded = false
    @State private var isEditing = false

    private var current: HealthEvent? { session.health.current(for: plant) }
    private var history: [HealthEvent] { session.health.history(for: plant) }

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            PlantHealthBar(event: current)

            if current == nil, session.health.hasLoaded(plant) {
                Text("Set a baseline only when you have evidence worth preserving. Planty will record later changes without rewriting it.")
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            if let failure = session.health.error(for: plant) {
                VStack(alignment: .leading, spacing: 6) {
                    Text(failure.errorDescription ?? "Health evidence could not be loaded.")
                        .font(.footnote)
                        .foregroundStyle(PlantyColor.orange)
                    Button("Try health again") {
                        Task { await session.health.load(plant) }
                    }
                    .font(.footnote.weight(.semibold))
                    .frame(minHeight: 44)
                }
            }

            Button(current == nil ? "Establish health baseline" : "Record a health correction") {
                isEditing = true
            }
            .buttonStyle(SecondaryButtonStyle())

            if !history.isEmpty {
                Divider().overlay(PlantyColor.quietDecoration)
                PlantyDisclosureHeader(
                    title: "Health history (\(history.count))",
                    icon: "waveform.path.ecg",
                    isExpanded: $historyExpanded,
                    color: PlantyColor.green
                )
                if historyExpanded {
                    VStack(alignment: .leading, spacing: 14) {
                        ForEach(history) { event in
                            HealthEventRow(event: event)
                            if event.id != history.last?.id {
                                Divider().overlay(PlantyColor.quietDecoration)
                            }
                        }
                    }
                }
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(border: PlantyColor.green.opacity(0.24))
        .sheet(isPresented: $isEditing) {
            HealthAdjustmentSheet(plant: plant, current: current) { change in
                await session.health.save(change, for: plant)
            }
        }
        .task(id: plant.slug) {
            await session.health.load(plant)
        }
    }
}

private struct HealthEventRow: View {
    let event: HealthEvent

    var body: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline, spacing: 8) {
                Text(changeLabel)
                    .font(.subheadline.weight(.bold))
                Spacer(minLength: 8)
                Text("\(number(event.score)) / 100")
                    .font(.subheadline.monospacedDigit().weight(.bold))
            }

            Text(event.rationale)
                .font(.subheadline)

            if let summary = event.evidence.summary?.nilIfBlank {
                Label(summary, systemImage: "doc.text.magnifyingglass")
                    .font(.footnote)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            if let references = referenceLine {
                Text(references)
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            if let model = event.evidence.modelVersion?.nilIfBlank {
                Text("Model \(model)")
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
            }

            Text(provenance)
                .font(.caption2)
                .foregroundStyle(PlantyColor.secondaryText)
        }
        .accessibilityElement(children: .combine)
    }

    private var changeLabel: String {
        guard !event.isBaseline else { return "Baseline" }
        guard let applied = event.appliedDelta else { return "Correction" }
        if let requested = event.requestedDelta, requested != applied {
            return "Applied \(signed(applied)); requested \(signed(requested))"
        }
        return "Correction \(signed(applied))"
    }

    private var referenceLine: String? {
        var parts: [String] = []
        add(event.evidence.photoIDs.count, singular: "photo", to: &parts)
        add(event.evidence.observationIDs.count, singular: "observation", to: &parts)
        add(event.evidence.readingIDs.count, singular: "reading", to: &parts)
        return parts.isEmpty ? nil : "Evidence: " + parts.joined(separator: ", ")
    }

    private var provenance: String {
        var parts = [event.source.healthLabel]
        if let actor = event.actor?.nilIfBlank { parts.append(actor) }
        parts.append(event.createdAt.formatted(date: .abbreviated, time: .shortened))
        return parts.joined(separator: " · ")
    }

    private func add(_ count: Int, singular: String, to parts: inout [String]) {
        guard count > 0 else { return }
        parts.append("\(count) \(singular)\(count == 1 ? "" : "s")")
    }

    private func signed(_ value: Double) -> String {
        value.formatted(.number.sign(strategy: .always()).precision(.fractionLength(0...1)))
    }

    private func number(_ value: Double) -> String {
        value.formatted(.number.precision(.fractionLength(0...1)))
    }
}

private extension ObservationSource {
    var healthLabel: String {
        switch self {
        case .app: "App"
        case .agent: "Agent"
        case .automation: "Automation"
        case .unknown: "Unknown source"
        }
    }
}

private struct HealthAdjustmentSheet: View {
    let plant: Plant
    let current: HealthEvent?
    let save: (NewHealthChange) async -> PlantyError?

    @Environment(\.dismiss) private var dismiss
    @State private var draft: HealthAdjustmentDraft
    @State private var idempotencyKey = UUID()
    @State private var action = AsyncSheetAction()

    init(
        plant: Plant,
        current: HealthEvent?,
        save: @escaping (NewHealthChange) async -> PlantyError?
    ) {
        self.plant = plant
        self.current = current
        self.save = save
        _draft = State(initialValue: HealthAdjustmentDraft(
            kind: current == nil ? .baseline : .delta
        ))
    }

    var body: some View {
        NavigationStack {
            Form {
                if let failure = action.error {
                    Section {
                        SheetErrorRow(
                            headline: "Health was not changed. Your evidence is still here.",
                            error: failure
                        )
                    }
                }

                Section {
                    if let current {
                        LabeledContent("Current", value: "\(number(current.score)) out of 100")
                    } else {
                        Text("Health is unknown until this first evidence-backed score is saved.")
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                    TextField(valuePrompt, text: $draft.value)
                        .keyboardType(.numbersAndPunctuation)
                        .accessibilityLabel(valueAccessibilityLabel)
                } header: {
                    Text(draft.kind == .baseline ? "Baseline from 0 to 100" : "Signed correction")
                } footer: {
                    Text(valueFooter)
                }

                Section("Why") {
                    TextField("Why should the score change?", text: $draft.rationale, axis: .vertical)
                        .lineLimit(2...6)
                }

                Section {
                    TextField("What did you look at or observe?", text: $draft.evidenceSummary, axis: .vertical)
                        .lineLimit(2...6)
                } header: {
                    Text("Evidence")
                } footer: {
                    Text("A concrete summary is required. This becomes part of the permanent health history.")
                }

                if current?.score == 0 {
                    Section {
                        Text("A score of zero records the evidence. It does not archive, delete, or change this plant's status.")
                            .font(.footnote.weight(.semibold))
                            .foregroundStyle(PlantyColor.red)
                    }
                }
            }
            .plantyPage()
            .navigationTitle(draft.kind == .baseline ? "Set health baseline" : "Correct health")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }.disabled(action.isRunning)
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await submit() } }
                        .disabled(action.isRunning || request == nil)
                }
            }
        }
        .interactiveDismissDisabled(action.isRunning)
        .onChange(of: draft) { _, _ in idempotencyKey = UUID() }
    }

    private var request: NewHealthChange? {
        draft.request(idempotencyKey: idempotencyKey)
    }

    private var valuePrompt: String {
        draft.kind == .baseline ? "For example, 75" : "For example, +5 or -12.5"
    }

    private var valueAccessibilityLabel: String {
        draft.kind == .baseline ? "Health baseline" : "Signed health correction"
    }

    private var valueFooter: String {
        draft.kind == .baseline
            ? "Zero means the evidence supports zero health; 100 means evidence supports ideal condition."
            : "The service clamps the result between zero and 100 and records both requested and applied change."
    }

    private func submit() async {
        guard let request else { return }
        if await action.perform({ await save(request) }) { dismiss() }
    }

    private func number(_ value: Double) -> String {
        value.formatted(.number.precision(.fractionLength(0...1)))
    }
}
