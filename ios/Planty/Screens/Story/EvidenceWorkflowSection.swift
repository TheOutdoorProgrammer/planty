import SwiftUI

struct EvidenceWorkflowSection: View {
    let plant: Plant
    let photos: [Photo]
    let observations: [PlantObservation]
    let record: (ObservationKind, String?) async -> Result<PlantObservation, PlantyError>

    @Environment(AppSession.self) private var session
    @State private var proposesRecheck = false

    private var windows: [EvidenceWindow] { session.evidenceWorkflows.windows(for: plant) }
    private var guardrails: [EvidenceWindow] { session.evidenceWorkflows.guardrails(for: plant) }

    var body: some View {
        VStack(alignment: .leading, spacing: 14) {
            SectionHeading("Care change follow-ups", detail: "Compare a baseline photo with what changed after a real care action.")

            ForEach(guardrails) { window in
                GuardrailCard(window: window, plant: plant)
            }

            if windows.isEmpty {
                Text(photos.isEmpty
                    ? "A before-and-after check needs a baseline photo. This story has none to reference."
                    : "No care change is being tracked for this plant.")
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
            } else {
                ForEach(windows) { window in
                    RecheckCard(
                        window: window,
                        plant: plant,
                        photos: photos,
                        observations: observations,
                        record: record
                    )
                }
            }

            Button("Track a care change") { proposesRecheck = true }
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
    let record: (ObservationKind, String?) async -> Result<PlantObservation, PlantyError>
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
            RecheckActionSheet(
                window: window,
                plant: plant,
                photos: photos,
                observations: observations,
                record: record
            )
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
        switch window.status { case .proposed: "Choose the care record"; case .active: "Attach follow-up photo"; case .ready: "Conclude follow-up"; default: "Review" }
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
                Section("Care action") { Picker("What changed", selection: $kind) { ForEach(kinds, id: \.self) { Text($0.label).tag($0) } } }
                Section("Baseline photo") {
                    Picker("Photo", selection: $baselineID) {
                        Text("Choose a photo").tag(UUID?.none)
                        ForEach(photos.sorted { $0.takenAt > $1.takenAt }) { photo in
                            Text(photo.takenAt.formatted(date: .abbreviated, time: .shortened)).tag(Optional(photo.id))
                        }
                    }
                }
                Section {
                    TextField("How to take the comparison photo", text: $instruction, axis: .vertical)
                        .lineLimit(2...5)
                } header: {
                    Text("Photo to take later")
                } footer: {
                    Text("This is guidance for you when the follow-up window opens. Planty cannot take the photo itself.")
                }
                Section("Bounded window") {
                    LabeledContent("Earliest", value: bounds.0.formatted(date: .abbreviated, time: .shortened))
                    LabeledContent("Latest", value: bounds.1.formatted(date: .abbreviated, time: .shortened))
                }
            }
            .plantyPage().navigationTitle("Track a care change").navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("Create") { Task { await submit() } }.disabled(baselineID == nil || instruction.cleaned.isEmpty) }
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
    let record: (ObservationKind, String?) async -> Result<PlantObservation, PlantyError>
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var selectedID: UUID?
    @State private var outcome: EvidenceWindowOutcome = .improved
    @State private var conclusion = ""
    @State private var failure: PlantyError?
    @State private var newlyRecorded: PlantObservation?
    @State private var logsIntervention = false
    @State private var confirmsCancellation = false

    var body: some View {
        NavigationStack {
            Form {
                if let failure { Section { SheetErrorRow(headline: "The follow-up was not changed.", error: failure) } }
                if window.status == .proposed {
                    Section("Recorded care action") {
                        let choices = matchingObservations
                        if choices.isEmpty {
                            Text("There is no \(window.interventionKind.careActionNoun) entry to attach yet. Log that care action, then Planty can start the before-and-after clock.")
                                .font(.subheadline)
                                .foregroundStyle(PlantyColor.secondaryText)
                            Button("Log \(window.interventionKind.careActionNoun)") {
                                logsIntervention = true
                            }
                        } else {
                            evidencePicker(choices.map { ($0.id, $0.occurredAt) })
                        }
                    }
                } else if window.status == .active {
                    Section("Review photo") {
                        evidencePicker(matchingReviewPhotos.map { ($0.id, $0.takenAt) })
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
                if window.status == .proposed || window.status == .active || window.status == .ready {
                    Section {
                        Button("Cancel this follow-up", role: .destructive) {
                            confirmsCancellation = true
                        }
                    } footer: {
                        Text("Use this when the planned care action changed. Existing photos and care records stay in the plant story.")
                    }
                }
            }
            .plantyPage().navigationTitle(window.status.label).navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Close") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await submit() } }
                        .disabled(window.status == .ready ? conclusion.cleaned.isEmpty : !selectionIsValid)
                }
            }
        }
        .confirmationDialog(
            "Cancel this care follow-up?",
            isPresented: $confirmsCancellation,
            titleVisibility: .visible
        ) {
            Button("Cancel follow-up", role: .destructive) {
                Task { await cancelFollowUp() }
            }
            Button("Keep follow-up", role: .cancel) {}
        } message: {
            Text("Planty will stop waiting for \(window.interventionKind.careActionNoun) and a comparison photo. Nothing already recorded will be deleted.")
        }
        .sheet(isPresented: $logsIntervention) {
            CareLogSheet(
                plantName: plant.commonName,
                initialKind: window.interventionKind,
                fixedKind: window.interventionKind
            ) { _, note in
                switch await record(window.interventionKind, note) {
                case .success(let observation) where observation.kind == window.interventionKind:
                    newlyRecorded = observation
                    selectedID = observation.id
                    return nil
                case .success:
                    failure = incompatibleInterventionError
                    selectedID = nil
                    return nil
                case .failure(let error):
                    return error
                }
            }
            .presentationDetents([.medium, .large])
        }
    }

    private var matchingObservations: [PlantObservation] {
        let baselineAt = photos
            .filter { window.baseline.map(\.id).contains($0.id) }
            .map(\.takenAt)
            .max()
        return ([newlyRecorded].compactMap { $0 } + observations)
            .filter {
                guard $0.kind == window.interventionKind else { return false }
                guard let baselineAt else { return true }
                return $0.occurredAt >= baselineAt
            }
            .reduce(into: [UUID: PlantObservation]()) { $0[$1.id] = $1 }
            .values
            .sorted { $0.occurredAt > $1.occurredAt }
    }

    private var matchingReviewPhotos: [Photo] {
        photos.filter {
            !window.baseline.map(\.id).contains($0.id) &&
                $0.takenAt >= window.earliestReviewAt &&
                $0.takenAt <= window.latestReviewAt
        }
    }

    private var selectionIsValid: Bool {
        guard let selectedID else { return false }
        if window.status == .proposed {
            return matchingObservations.contains { $0.id == selectedID }
        }
        if window.status == .active {
            return matchingReviewPhotos.contains { $0.id == selectedID }
        }
        return false
    }

    private var incompatibleInterventionError: PlantyError {
        .server(
            status: 422,
            message: "This follow-up is tracking \(window.interventionKind.careActionNoun). Choose or log a matching care record."
        )
    }

    private func evidencePicker(_ choices: [(UUID, Date)]) -> some View {
        Picker("Evidence", selection: $selectedID) {
            Text("Choose a ledger record").tag(UUID?.none)
            ForEach(choices, id: \.0) { id, date in Text(date.formatted(date: .abbreviated, time: .shortened)).tag(Optional(id)) }
        }
    }

    private func submit() async {
        if window.status == .proposed {
            guard selectionIsValid, let selectedID else {
                failure = incompatibleInterventionError
                self.selectedID = nil
                return
            }
            failure = await session.evidenceWorkflows.start(window, observationID: selectedID)
        } else if window.status == .active {
            guard selectionIsValid, let selectedID else { return }
            failure = await session.evidenceWorkflows.review(window, evidence: [.init(plantID: plant.id, kind: .photo, id: selectedID)])
        } else if window.status == .ready {
            failure = await session.evidenceWorkflows.conclude(window, outcome: outcome, conclusion: conclusion)
        }
        if failure == nil { dismiss() }
    }

    private func cancelFollowUp() async {
        failure = await session.evidenceWorkflows.cancel(
            window,
            reason: "Owner cancelled the \(window.interventionKind.careActionNoun) follow-up."
        )
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
