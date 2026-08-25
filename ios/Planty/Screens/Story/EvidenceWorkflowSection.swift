import SwiftUI

struct EvidenceWorkflowSection: View {
    let plant: Plant
    let photos: [Photo]
    let observations: [PlantObservation]

    @Environment(AppSession.self) private var session
    @State private var proposesRecheck = false

    private var windows: [EvidenceWindow] { session.evidenceWorkflows.windows(for: plant) }
    private var guardrails: [EvidenceWindow] { session.evidenceWorkflows.guardrails(for: plant) }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            SectionHeading("Evidence rechecks", detail: "Compare the same question before and after one recorded intervention.")

            ForEach(guardrails) { window in
                GuardrailCard(window: window, plant: plant)
            }

            if windows.isEmpty {
                Text(photos.isEmpty
                    ? "A recheck needs a baseline photo. This story has none to reference."
                    : "No recheck is known for this plant.")
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            } else {
                ForEach(windows) { window in
                    RecheckCard(window: window, plant: plant, photos: photos, observations: observations)
                }
            }

            Button("Propose a photo recheck") { proposesRecheck = true }
                .buttonStyle(SecondaryButtonStyle())
                .disabled(photos.isEmpty)
        }
        .sheet(isPresented: $proposesRecheck) {
            RecheckProposalSheet(plant: plant, photos: photos)
        }
        .task(id: plant.slug) {
            async let rechecks: Void = session.evidenceWorkflows.loadRechecks(for: plant)
            async let guardrails: Void = session.evidenceWorkflows.loadGuardrails(for: plant)
            _ = await (rechecks, guardrails)
        }
    }
}

private struct GuardrailCard: View {
    let window: EvidenceWindow
    let plant: Plant
    @State private var overrides = false

    var body: some View {
        if let guardrail = window.guardrail {
            VStack(alignment: .leading, spacing: 8) {
                Label("Do Not Disturb", systemImage: "hand.raised.fill")
                    .font(.headline)
                    .foregroundStyle(PlantyColor.orange)
                Text(guardrail.reason).font(.subheadline)
                Text("Avoid: \(guardrail.conflictingKinds.map(\.label).joined(separator: ", "))")
                    .font(.caption.weight(.semibold))
                if !guardrail.redFlags.isEmpty {
                    Text("Red flags that justify reassessment: \(guardrail.redFlags.joined(separator: "; ")).")
                        .font(.caption)
                        .foregroundStyle(PlantyColor.secondaryText)
                }
                Button("Record an audited override") { overrides = true }
                    .font(.subheadline.weight(.semibold))
            }
            .plantyCard(border: PlantyColor.orange.opacity(0.3), padding: 14)
            .sheet(isPresented: $overrides) {
                GuardrailOverrideSheet(window: window, plant: plant, guardrail: guardrail)
            }
        }
    }
}

private struct RecheckCard: View {
    let window: EvidenceWindow
    let plant: Plant
    let photos: [Photo]
    let observations: [PlantObservation]
    @State private var acts = false

    var body: some View {
        VStack(alignment: .leading, spacing: 9) {
            HStack {
                Label(window.interventionKind.label, systemImage: "camera.metering.center.weighted")
                    .font(.headline)
                Spacer()
                Text(window.status.label).font(.caption.weight(.semibold)).foregroundStyle(PlantyColor.cyan)
            }
            Text("Review between \(window.earliestReviewAt.formatted(date: .abbreviated, time: .shortened)) and \(window.latestReviewAt.formatted(date: .abbreviated, time: .shortened)).")
                .font(.caption)
                .foregroundStyle(PlantyColor.secondaryText)
            comparison
            if let conclusion = window.conclusion?.nilIfBlank {
                Text(conclusion).font(.subheadline)
            }
            if let reason = window.confoundReason?.nilIfBlank {
                Label("Confounded: \(reason)", systemImage: "exclamationmark.triangle.fill")
                    .font(.caption).foregroundStyle(PlantyColor.orange)
            }
            if window.status == .proposed || window.status == .active || window.status == .ready {
                Button(actionLabel) { acts = true }.buttonStyle(SecondaryButtonStyle())
            }
        }
        .plantyCard(border: PlantyColor.cyan.opacity(0.22), padding: 14)
        .sheet(isPresented: $acts) {
            RecheckActionSheet(window: window, plant: plant, photos: photos, observations: observations)
        }
    }

    @ViewBuilder private var comparison: some View {
        let ids = Set((window.baseline + window.review).filter { $0.kind == .photo }.map(\.id))
        let matched = photos.filter { ids.contains($0.id) }
        if window.review.contains(where: { $0.kind == .photo }) {
            if matched.count >= 2 {
                NavigationLink("Compare baseline and review photos") {
                    PhotoComparisonScreen(plant: plant, comparison: PhotoComparison(matched))
                }
            } else {
                Label("Comparison unavailable: a referenced image is not in the loaded story.", systemImage: "photo.badge.exclamationmark")
                    .font(.caption).foregroundStyle(PlantyColor.orange)
            }
        }
    }

    private var actionLabel: String {
        switch window.status { case .proposed: "Start with intervention record"; case .active: "Attach review evidence"; case .ready: "Conclude recheck"; default: "Review" }
    }
}

private struct RecheckProposalSheet: View {
    let plant: Plant
    let photos: [Photo]
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var kind: ObservationKind = .watered
    @State private var baselineID: UUID?
    @State private var instruction = "Take a whole-plant photo from the same angle and lighting."
    @State private var failure: PlantyError?

    private let kinds: [ObservationKind] = [.watered, .moved, .repotted, .fertilized, .pruned]

    var body: some View {
        NavigationStack {
            Form {
                if let failure { Section { SheetErrorRow(headline: "The recheck was not proposed.", error: failure) } }
                Section("Intervention") { Picker("Kind", selection: $kind) { ForEach(kinds, id: \.self) { Text($0.label).tag($0) } } }
                Section("Baseline photo") {
                    Picker("Photo", selection: $baselineID) {
                        Text("Choose a photo").tag(UUID?.none)
                        ForEach(photos.sorted { $0.takenAt > $1.takenAt }) { photo in
                            Text(photo.takenAt.formatted(date: .abbreviated, time: .shortened)).tag(Optional(photo.id))
                        }
                    }
                }
                Section("Expected review evidence") { TextField("Photo instruction", text: $instruction, axis: .vertical).lineLimit(2...5) }
                Section("Bounded window") {
                    LabeledContent("Earliest", value: bounds.0.formatted(date: .abbreviated, time: .shortened))
                    LabeledContent("Latest", value: bounds.1.formatted(date: .abbreviated, time: .shortened))
                }
            }
            .plantyPage().navigationTitle("Propose recheck").navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("Propose") { Task { await submit() } }.disabled(baselineID == nil || instruction.cleaned.isEmpty) }
            }
        }
    }

    private var bounds: (Date, Date) {
        let now = Date()
        let days: (Double, Double) = switch kind {
        case .watered: (1.0 / 24.0, 3); case .moved, .pruned: (1, 14); case .repotted: (2, 21); case .fertilized: (2, 14); default: (1, 3)
        }
        return (now.addingTimeInterval(days.0 * 86_400 + 300), now.addingTimeInterval(days.1 * 86_400 - 300))
    }

    private func submit() async {
        guard let baselineID else { return }
        let bounds = bounds
        let proposal = RecheckProposal(
            interventionKind: kind,
            baseline: [.init(plantID: plant.id, kind: .photo, id: baselineID)],
            expected: [.init(plantID: plant.id, kind: .photo, instruction: instruction.cleaned)],
            earliestReviewAt: bounds.0, latestReviewAt: bounds.1, actor: "owner"
        )
        failure = await session.evidenceWorkflows.proposeRecheck(for: plant, proposal: proposal)
        if failure == nil { dismiss() }
    }
}

private struct RecheckActionSheet: View {
    let window: EvidenceWindow; let plant: Plant; let photos: [Photo]; let observations: [PlantObservation]
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var selectedID: UUID?
    @State private var outcome: EvidenceWindowOutcome = .improved
    @State private var conclusion = ""
    @State private var failure: PlantyError?

    var body: some View {
        NavigationStack {
            Form {
                if let failure { Section { SheetErrorRow(headline: "The recheck was not changed.", error: failure) } }
                if window.status == .proposed {
                    Section("Recorded intervention") {
                        evidencePicker(observations.filter {
                            $0.kind == window.interventionKind && $0.createdAt >= window.createdAt
                        }.map { ($0.id, $0.occurredAt) })
                    }
                } else if window.status == .active {
                    Section("Review photo") {
                        evidencePicker(photos.filter {
                            !window.baseline.map(\.id).contains($0.id) &&
                                $0.takenAt >= window.earliestReviewAt &&
                                $0.takenAt <= window.latestReviewAt
                        }.map { ($0.id, $0.takenAt) })
                    }
                } else if window.status == .ready {
                    Section("Outcome") {
                        Picker("Result", selection: $outcome) {
                            Text("Improved").tag(EvidenceWindowOutcome.improved)
                            Text("Unchanged").tag(EvidenceWindowOutcome.unchanged)
                            Text("Worsened").tag(EvidenceWindowOutcome.worsened)
                            Text("Insufficient evidence").tag(EvidenceWindowOutcome.insufficientEvidence)
                        }
                        TextField("Evidence-backed conclusion", text: $conclusion, axis: .vertical).lineLimit(2...6)
                    }
                }
            }
            .plantyPage().navigationTitle(window.status.label).navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("Save") { Task { await submit() } }.disabled(window.status == .ready ? conclusion.cleaned.isEmpty : selectedID == nil) }
            }
        }
    }

    private func evidencePicker(_ choices: [(UUID, Date)]) -> some View {
        Picker("Evidence", selection: $selectedID) {
            Text("Choose a ledger record").tag(UUID?.none)
            ForEach(choices, id: \.0) { id, date in Text(date.formatted(date: .abbreviated, time: .shortened)).tag(Optional(id)) }
        }
    }

    private func submit() async {
        if window.status == .proposed, let selectedID { failure = await session.evidenceWorkflows.start(window, observationID: selectedID) }
        else if window.status == .active, let selectedID { failure = await session.evidenceWorkflows.review(window, evidence: [.init(plantID: plant.id, kind: .photo, id: selectedID)]) }
        else if window.status == .ready { failure = await session.evidenceWorkflows.conclude(window, outcome: outcome, conclusion: conclusion) }
        if failure == nil { dismiss() }
    }
}

private struct GuardrailOverrideSheet: View {
    let window: EvidenceWindow; let plant: Plant; let guardrail: EvidenceGuardrail
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var kind: ObservationKind
    @State private var reason = ""
    @State private var failure: PlantyError?

    init(window: EvidenceWindow, plant: Plant, guardrail: EvidenceGuardrail) {
        self.window = window; self.plant = plant; self.guardrail = guardrail
        _kind = State(initialValue: guardrail.conflictingKinds.first ?? .unknown)
    }

    var body: some View {
        NavigationStack {
            Form {
                Text("This does not remove the guardrail. Planty records who overrode which conflict and why, and marks the evidence window confounded.")
                Picker("Conflicting action", selection: $kind) { ForEach(guardrail.conflictingKinds, id: \.self) { Text($0.label).tag($0) } }
                TextField("Why is this necessary now?", text: $reason, axis: .vertical).lineLimit(2...6)
                if let failure { SheetErrorRow(headline: "The override was not recorded.", error: failure) }
            }
            .plantyPage().navigationTitle("Audited override").navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("Record override") { Task { await submit() } }.disabled(reason.cleaned.isEmpty || kind == .unknown) }
            }
        }
    }

    private func submit() async {
        failure = await session.evidenceWorkflows.override(window, plant: plant, kind: kind, reason: reason)
        if failure == nil { dismiss() }
    }
}

private extension EvidenceWindowStatus {
    var label: String { rawValue.replacingOccurrences(of: "_", with: " ").capitalized }
}
