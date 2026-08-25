import SwiftUI

struct ExperimentListScreen: View {
    @Environment(AppSession.self) private var session
    @State private var proposes = false

    var body: some View {
        List {
            Section {
                Text("One named variable, explicit hold-constant rules, success criteria, and a bounded review window. These are evidence windows, not unattended automations.")
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
                Button("Propose household experiment") { proposes = true }
            }
            Section("Experiments") {
                if let error = session.evidenceWorkflows.error {
                    Text(error.errorDescription ?? "Experiments could not be loaded.").foregroundStyle(PlantyColor.orange)
                }
                if session.evidenceWorkflows.experiments.isEmpty {
                    Text("No household experiments are recorded.").foregroundStyle(PlantyColor.secondaryText)
                }
                ForEach(session.evidenceWorkflows.experiments) { window in
                    NavigationLink { ExperimentDetailScreen(windowID: window.id) } label: {
                        VStack(alignment: .leading, spacing: 4) {
                            Text(window.experiment?.title ?? "Experiment").font(.body.weight(.semibold))
                            Text("\(window.status.displayLabel) · \(window.plantIDs.count) plant\(window.plantIDs.count == 1 ? "" : "s")")
                                .font(.caption).foregroundStyle(PlantyColor.secondaryText)
                        }
                    }
                }
            }
        }
        .scrollContentBackground(.hidden).plantyPage()
        .navigationTitle("Household experiments").navigationBarTitleDisplayMode(.inline)
        .refreshable { await session.evidenceWorkflows.loadExperiments() }
        .task { await session.evidenceWorkflows.loadExperiments() }
        .sheet(isPresented: $proposes) { ExperimentProposalSheet(plants: session.library.plants) }
    }
}

private struct ExperimentDetailScreen: View {
    let windowID: UUID
    @Environment(AppSession.self) private var session
    @State private var acts = false

    private var window: EvidenceWindow? { session.evidenceWorkflows.window(id: windowID) }
    private var experiment: EvidenceExperiment? { window?.experiment }
    private var plants: [Plant] {
        guard let window else { return [] }
        return session.library.plants.filter { window.plantIDs.contains($0.id) }
    }

    var body: some View {
        List {
            if let window {
                Section("Evidence status") {
                    LabeledContent("Status", value: window.status.displayLabel)
                    LabeledContent("Review opens") { Text(window.earliestReviewAt.formatted(date: .abbreviated, time: .shortened)) }
                    LabeledContent("Review closes") { Text(window.latestReviewAt.formatted(date: .abbreviated, time: .shortened)) }
                    if let outcome = window.outcome { LabeledContent("Outcome", value: outcome.rawValue.replacingOccurrences(of: "_", with: " ").capitalized) }
                    if let conclusion = window.conclusion?.nilIfBlank { Text(conclusion) }
                    if window.status == .proposed || window.status == .active || window.status == .ready {
                        Button(actionLabel(window.status)) { acts = true }
                    }
                }
                if let experiment {
                    Section("Hypothesis") { Text(experiment.hypothesis) }
                    Section("Only variable") { LabeledContent(experiment.variableKind, value: experiment.variableValue) }
                    Section("Hold constant") { ForEach(experiment.holdConstantRules, id: \.self) { Label($0, systemImage: "equal.circle") } }
                    Section("Success criteria") { ForEach(experiment.successCriteria, id: \.self) { Label($0, systemImage: "checkmark.circle") } }
                }
                if let guardrail = window.guardrail {
                    Section("Do Not Disturb") {
                        Text(guardrail.reason)
                        Text("Avoid: \(guardrail.conflictingKinds.map(\.label).joined(separator: ", "))")
                    }
                }
                if let reason = window.confoundReason?.nilIfBlank {
                    Section("Confounded") { Text(reason).foregroundStyle(PlantyColor.orange) }
                }
            } else {
                ProgressView("Loading experiment…")
            }
        }
        .scrollContentBackground(.hidden).plantyPage()
        .navigationTitle(experiment?.title ?? "Experiment").navigationBarTitleDisplayMode(.inline)
        .task {
            await session.library.load()
            await session.evidenceWorkflows.loadExperiments()
            await session.evidenceWorkflows.loadLedgers(for: plants)
        }
        .sheet(isPresented: $acts) {
            if let window {
                ExperimentActionSheet(window: window, plants: plants)
            }
        }
    }

    private func actionLabel(_ status: EvidenceWindowStatus) -> String {
        switch status {
        case .proposed: "Start from a recorded intervention"
        case .active: "Attach review evidence"
        case .ready: "Conclude experiment"
        default: "Review experiment"
        }
    }
}

private struct ExperimentActionSheet: View {
    let window: EvidenceWindow
    let plants: [Plant]

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var interventionID: UUID?
    @State private var reviewIDs: [UUID: UUID] = [:]
    @State private var outcome: EvidenceWindowOutcome = .supported
    @State private var conclusion = ""
    @State private var failure: PlantyError?

    private var observations: [(Plant, PlantObservation)] {
        plants.flatMap { plant in
            session.evidenceWorkflows.ledger(for: plant).observations
                .filter { $0.kind == window.interventionKind && $0.createdAt >= window.createdAt }
                .map { (plant, $0) }
        }
        .sorted { $0.1.occurredAt > $1.1.occurredAt }
    }

    var body: some View {
        NavigationStack {
            Form {
                if let failure {
                    Section { SheetErrorRow(headline: "The experiment was not changed.", error: failure) }
                }
                if window.status == .proposed {
                    Section("Recorded intervention") {
                        Picker("Ledger entry", selection: $interventionID) {
                            Text("Choose a record").tag(UUID?.none)
                            ForEach(observations, id: \.1.id) { plant, observation in
                                Text("\(plant.commonName) · \(observation.occurredAt.formatted(date: .abbreviated, time: .shortened))")
                                    .tag(Optional(observation.id))
                            }
                        }
                        if observations.isEmpty {
                            Text("Record the intervention on one participating plant before starting. Planty will not invent a ledger entry.")
                                .font(.footnote)
                                .foregroundStyle(PlantyColor.orange)
                        }
                    }
                } else if window.status == .active {
                    Section {
                        if Date() < window.earliestReviewAt {
                            Text("The review window has not opened yet.")
                                .foregroundStyle(PlantyColor.orange)
                        } else if Date() > window.latestReviewAt {
                            Text("The review window has closed. Preserve that fact instead of attaching late evidence.")
                                .foregroundStyle(PlantyColor.orange)
                        }
                    } footer: {
                        Text("Each expected plant needs its own ledger-backed review photo.")
                    }
                    ForEach(window.expected, id: \.plantID) { expected in
                        if let plant = plants.first(where: { $0.id == expected.plantID }) {
                            Section(plant.commonName) {
                                Picker(expected.instruction, selection: reviewBinding(for: plant.id)) {
                                    Text("Choose a review photo").tag(UUID?.none)
                                    ForEach(reviewPhotos(for: plant)) { photo in
                                        Text(photo.takenAt.formatted(date: .abbreviated, time: .shortened))
                                            .tag(Optional(photo.id))
                                    }
                                }
                            }
                        }
                    }
                } else if window.status == .ready {
                    Section("Evidence-backed outcome") {
                        Picker("Outcome", selection: $outcome) {
                            Text("Hypothesis supported").tag(EvidenceWindowOutcome.supported)
                            Text("Hypothesis not supported").tag(EvidenceWindowOutcome.notSupported)
                            Text("Inconclusive").tag(EvidenceWindowOutcome.inconclusive)
                            Text("Stopped for safety").tag(EvidenceWindowOutcome.stoppedForSafety)
                        }
                        TextField("What did the evidence show?", text: $conclusion, axis: .vertical)
                            .lineLimit(3...7)
                    }
                }
            }
            .plantyPage()
            .navigationTitle(window.status.displayLabel)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await submit() } }
                        .disabled(!canSubmit)
                }
            }
        }
    }

    private var canSubmit: Bool {
        switch window.status {
        case .proposed: interventionID != nil
        case .active:
            Date() >= window.earliestReviewAt && Date() <= window.latestReviewAt &&
                window.expected.allSatisfy { reviewIDs[$0.plantID] != nil }
        case .ready: !conclusion.cleaned.isEmpty
        default: false
        }
    }

    private func reviewPhotos(for plant: Plant) -> [Photo] {
        let baseline = Set(window.baseline.map(\.id))
        return session.evidenceWorkflows.ledger(for: plant).photos.filter {
            !baseline.contains($0.id) && $0.takenAt >= window.earliestReviewAt &&
                $0.takenAt <= window.latestReviewAt
        }
        .sorted { $0.takenAt > $1.takenAt }
    }

    private func reviewBinding(for plantID: UUID) -> Binding<UUID?> {
        Binding(
            get: { reviewIDs[plantID] },
            set: { reviewIDs[plantID] = $0 }
        )
    }

    private func submit() async {
        if window.status == .proposed, let interventionID {
            failure = await session.evidenceWorkflows.start(window, observationID: interventionID)
        } else if window.status == .active {
            let references = window.expected.compactMap { expected -> EvidenceReferenceRequest? in
                guard let id = reviewIDs[expected.plantID] else { return nil }
                return EvidenceReferenceRequest(plantID: expected.plantID, kind: expected.kind, id: id)
            }
            failure = await session.evidenceWorkflows.review(window, evidence: references)
        } else if window.status == .ready {
            failure = await session.evidenceWorkflows.conclude(window, outcome: outcome, conclusion: conclusion)
        }
        if failure == nil { dismiss() }
    }
}

private struct ExperimentProposalSheet: View {
    let plants: [Plant]
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var selected: Set<UUID> = []
    @State private var kind: ObservationKind = .moved
    @State private var title = ""
    @State private var hypothesis = ""
    @State private var variableKind = ""
    @State private var variableValue = ""
    @State private var constants = ""
    @State private var criteria = ""
    @State private var failure: PlantyError?

    private let kinds: [ObservationKind] = [.watered, .moved, .repotted, .fertilized, .pruned]
    private var chosen: [Plant] { plants.filter { selected.contains($0.id) } }
    private var missingPhotos: [Plant] { chosen.filter { session.evidenceWorkflows.latestPhotos[$0.id] == nil } }

    var body: some View {
        NavigationStack {
            Form {
                if let failure { Section { SheetErrorRow(headline: "The experiment was not proposed.", error: failure) } }
                Section("Plants") {
                    ForEach(plants.filter { !$0.status.isRetired }) { plant in
                        Button { if !selected.insert(plant.id).inserted { selected.remove(plant.id) } } label: {
                            HStack { Text(plant.commonName).foregroundStyle(.primary); Spacer(); if selected.contains(plant.id) { Image(systemName: "checkmark") } }
                        }.buttonStyle(.plain)
                    }
                    if !missingPhotos.isEmpty { Text("Missing baseline photo: \(missingPhotos.map(\.commonName).joined(separator: ", "))").font(.caption).foregroundStyle(PlantyColor.orange) }
                }
                Section("Question") {
                    TextField("Experiment title", text: $title)
                    TextField("Hypothesis", text: $hypothesis, axis: .vertical).lineLimit(2...5)
                }
                Section("One variable") {
                    Picker("Intervention", selection: $kind) { ForEach(kinds, id: \.self) { Text($0.label).tag($0) } }
                    TextField("Variable name", text: $variableKind)
                    TextField("Value or change", text: $variableValue)
                }
                Section("Hold constant") { TextField("Separate rules with semicolons", text: $constants, axis: .vertical).lineLimit(2...5) }
                Section("Success criteria") { TextField("Separate criteria with semicolons", text: $criteria, axis: .vertical).lineLimit(2...5) }
                Section("Bounded review") {
                    LabeledContent("Earliest", value: bounds.0.formatted(date: .abbreviated, time: .shortened))
                    LabeledContent("Latest", value: bounds.1.formatted(date: .abbreviated, time: .shortened))
                }
            }
            .plantyPage().navigationTitle("Propose experiment").navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("Propose") { Task { await submit() } }.disabled(!isValid) }
            }
            .task { await session.evidenceWorkflows.loadLatestPhotos(for: plants.filter { !$0.status.isRetired }) }
        }
    }

    private var rules: [String] { constants.split(separator: ";").map { String($0).cleaned }.filter { !$0.isEmpty } }
    private var success: [String] { criteria.split(separator: ";").map { String($0).cleaned }.filter { !$0.isEmpty } }
    private var isValid: Bool { !chosen.isEmpty && missingPhotos.isEmpty && !title.cleaned.isEmpty && !hypothesis.cleaned.isEmpty && !variableKind.cleaned.isEmpty && !variableValue.cleaned.isEmpty && !rules.isEmpty && !success.isEmpty }
    private var bounds: (Date, Date) {
        let now = Date(); let days: (Double, Double) = switch kind {
        case .watered: (1.0 / 24.0, 3); case .moved, .pruned: (1, 14); case .repotted: (2, 21); case .fertilized: (2, 14); default: (1, 3)
        }
        return (now.addingTimeInterval(days.0 * 86_400 + 300), now.addingTimeInterval(days.1 * 86_400 - 300))
    }

    private func submit() async {
        let bounds = bounds
        let baseline = chosen.compactMap { plant -> EvidenceReferenceRequest? in
            guard let photo = session.evidenceWorkflows.latestPhotos[plant.id] else { return nil }
            return .init(plantID: plant.id, kind: .photo, id: photo.id)
        }
        let expected = chosen.map { EvidenceExpectation(plantID: $0.id, kind: .photo, instruction: "Take a comparable whole-plant photo from the same angle and lighting.") }
        let proposal = ExperimentProposal(
            plantIDs: chosen.map(\.id), interventionKind: kind, baseline: baseline, expected: expected,
            earliestReviewAt: bounds.0, latestReviewAt: bounds.1, actor: "owner",
            title: title.cleaned, hypothesis: hypothesis.cleaned, variableKind: variableKind.cleaned,
            variableValue: variableValue.cleaned, holdConstantRules: rules, successCriteria: success
        )
        failure = await session.evidenceWorkflows.proposeExperiment(proposal)
        if failure == nil { dismiss() }
    }
}

private extension EvidenceWindowStatus {
    var displayLabel: String { rawValue.replacingOccurrences(of: "_", with: " ").capitalized }
}
