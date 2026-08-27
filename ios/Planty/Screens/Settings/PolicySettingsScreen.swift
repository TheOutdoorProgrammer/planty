import SwiftUI

struct PolicySettingsScreen: View {
    @Environment(AppSession.self) private var session
    @State private var editing: OPAPolicy?
    @State private var isCreating = false

    var body: some View {
        List {
            if let error = session.policies.error {
                Text(error.errorDescription ?? "Policies could not be loaded.")
                    .foregroundStyle(PlantyColor.orange)
            }
            Section {
                NavigationLink {
                    PolicyReferenceScreen()
                } label: {
                    Label("Inputs, outputs, and safety", systemImage: "book.pages.fill")
                }
            } footer: {
                Text("The service supplies this reference, so field names cannot drift from reality.")
            }
            Section("Policies") {
                if !session.policies.hasLoaded {
                    HStack { ProgressView(); Text("Loading policies…") }
                } else if session.policies.policies.isEmpty {
                    ContentUnavailableView(
                        "No policies yet",
                        systemImage: "checkmark.shield",
                        description: Text(
                            "Start with the example, preview it against a real plant, then enable it."
                        )
                    )
                }
                ForEach(session.policies.policies) { policy in
                    Button { editing = policy } label: {
                        PolicyRow(policy: policy)
                    }
                    .buttonStyle(.plain)
                }
            }
            if !session.policies.evaluations.isEmpty {
                Section("Recent decisions") {
                    ForEach(session.policies.evaluations.prefix(20)) { evaluation in
                        PolicyEvaluationRow(evaluation: evaluation)
                    }
                }
            }
        }
        .scrollContentBackground(.hidden)
        .plantyPage()
        .navigationTitle("OPA policies")
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button { isCreating = true } label: { Image(systemName: "plus") }
                    .accessibilityLabel("New policy")
            }
        }
        .task {
            async let policies: Void = session.policies.load()
            async let plants: Void = session.library.load()
            _ = await (policies, plants)
        }
        .sheet(isPresented: $isCreating) {
            NavigationStack { PolicyEditorScreen(policy: nil) }
        }
        .sheet(item: $editing) { policy in
            NavigationStack { PolicyEditorScreen(policy: policy) }
        }
    }
}

private struct PolicyRow: View {
    let policy: OPAPolicy

    var body: some View {
        HStack(spacing: 12) {
            Image(systemName: policy.enabled ? "checkmark.shield.fill" : "shield.slash")
                .font(.title3)
                .foregroundStyle(policy.enabled ? PlantyColor.green : PlantyColor.secondaryText)
            VStack(alignment: .leading, spacing: 3) {
                Text(policy.name).font(.headline)
                Text("v\(policy.version) · \(policy.mode.label) · \(policy.enabled ? "enabled" : "disabled")")
                    .font(.caption)
                    .foregroundStyle(PlantyColor.secondaryText)
                if !policy.description.cleaned.isEmpty {
                    Text(policy.description).font(.caption).foregroundStyle(PlantyColor.secondaryText)
                }
            }
            Spacer()
            Image(systemName: "chevron.right").foregroundStyle(PlantyColor.secondaryText)
        }
        .contentShape(Rectangle())
    }
}

private struct PolicyEditorScreen: View {
    let policy: OPAPolicy?

    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var draft: PolicyDraft
    @State private var selectedPlantSlug = ""
    @State private var preview: PolicyPreview?
    @State private var isPreviewing = false
    @State private var confirmsDelete = false

    init(policy: OPAPolicy?) {
        self.policy = policy
        _draft = State(initialValue: policy.map {
            PolicyDraft(
                name: $0.name,
                description: $0.description,
                source: $0.source,
                mode: $0.mode,
                enabled: $0.enabled
            )
        } ?? .new(reference: nil))
    }

    var body: some View {
        Form {
            Section("Identity") {
                TextField("Policy name", text: $draft.name)
                TextField("What this decides", text: $draft.description, axis: .vertical)
            }
            Section {
                Picker("Mode", selection: $draft.mode) {
                    Text("Advisory").tag(PolicyMode.advisory)
                    Text("Enforce").tag(PolicyMode.enforce)
                }
                .pickerStyle(.segmented)
                Toggle("Enabled in daily care", isOn: $draft.enabled)
            } footer: {
                Text(draft.mode == .enforce
                     ? "Enforce may notify, adjust health from fresh evidence, or run an opted-in fan. " +
                        "It still cannot water, mist, or move a plant."
                     : "Advisory records the decision and gives it to agents, but changes nothing physical.")
            }
            Section {
                TextEditor(text: $draft.source)
                    .font(.system(.caption, design: .monospaced))
                    .frame(minHeight: 360)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                HStack {
                    Text("\(draft.source.lengthOfBytes(using: .utf8)) / \(PolicyDraft.maximumBytes) bytes")
                        .font(.caption2)
                        .foregroundStyle(PlantyColor.secondaryText)
                    Spacer()
                    NavigationLink("Rule reference") { PolicyReferenceScreen() }
                }
            } header: {
                Text("Rego v1")
            } footer: {
                Text(
                    "Define one object at data.planty.decision. Network, clock, random, UUID, " +
                    "and runtime built-ins are blocked."
                )
            }
            previewSection
            if let error = session.policies.error {
                Section { SheetErrorRow(headline: "The policy was not accepted.", error: error) }
            }
            Section {
                Button(session.policies.isWriting ? "Saving…" : "Compile and save") {
                    Task { await save() }
                }
                .disabled(!draft.isValid || session.policies.isWriting)
            }
            if policy != nil {
                Section {
                    Button("Delete policy", role: .destructive) { confirmsDelete = true }
                } footer: {
                    Text("Evaluation history stays for audit. The policy stops running immediately.")
                }
            }
        }
        .scrollContentBackground(.hidden)
        .plantyPage()
        .navigationTitle(policy == nil ? "New policy" : policy?.name ?? "Policy")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar { ToolbarItem(placement: .cancellationAction) { Button("Close") { dismiss() } } }
        .task {
            if session.policies.reference == nil { await session.policies.load() }
            if policy == nil,
               draft.source == PolicyDraft.fallbackExample,
               let example = session.policies.reference?.example {
                draft.source = example
            }
            if selectedPlantSlug.isEmpty {
                selectedPlantSlug = session.library.plants.first?.slug ?? ""
            }
        }
        .confirmationDialog("Delete this policy?", isPresented: $confirmsDelete) {
            Button("Delete policy", role: .destructive) { Task { await deletePolicy() } }
            Button("Keep policy", role: .cancel) {}
        }
    }

    @ViewBuilder
    private var previewSection: some View {
        Section {
            Picker("Test plant", selection: $selectedPlantSlug) {
                ForEach(session.library.plants.filter { !$0.status.isRetired }) { plant in
                    Text(plant.commonName).tag(plant.slug)
                }
            }
            Button(isPreviewing ? "Evaluating…" : "Preview with current evidence") {
                Task { await runPreview() }
            }
            .disabled(!draft.isValid || selectedPlantSlug.isEmpty || isPreviewing)
            if let preview {
                PolicyDecisionView(decision: preview.decision)
                LabeledContent("Evaluation", value: String(format: "%.2f ms", preview.durationMS))
                    .font(.caption)
            }
        } header: {
            Text("Safe preview")
        } footer: {
            Text(
                "Preview builds the production input but never persists, notifies, changes health, " +
                "or controls a device."
            )
        }
    }

    private func runPreview() async {
        isPreviewing = true
        defer { isPreviewing = false }
        preview = await session.policies.preview(draft, plantSlug: selectedPlantSlug)
    }

    private func save() async {
        if await session.policies.save(id: policy?.id, draft: draft) != nil { dismiss() }
    }

    private func deletePolicy() async {
        guard let policy else { return }
        if await session.policies.delete(policy) { dismiss() }
    }
}

private struct PolicyDecisionView: View {
    let decision: PolicyDecision

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Label(decision.summary, systemImage: "sparkles")
                .font(.headline)
            ForEach(decision.signals.filter(\.active)) { signal in
                HStack(alignment: .top) {
                    Image(systemName: signal.kind.symbol).foregroundStyle(signal.severity.color)
                    VStack(alignment: .leading) {
                        Text(signal.kind.label).font(.subheadline.weight(.semibold))
                        Text(signal.reason).font(.caption).foregroundStyle(PlantyColor.secondaryText)
                    }
                }
            }
            if let health = decision.health {
                Label(
                    "Health \(health.delta, format: .number.sign(strategy: .always()))",
                    systemImage: "heart.text.square"
                )
            }
            if !decision.fanRuns.isEmpty {
                Label("\(decision.fanRuns.count) bounded fan run requested", systemImage: "fan.fill")
            }
            if !decision.notifications.isEmpty {
                Label("\(decision.notifications.count) notification", systemImage: "bell.badge.fill")
            }
        }
        .padding(.vertical, 6)
    }
}

private struct PolicyEvaluationRow: View {
    let evaluation: PolicyEvaluation

    var body: some View {
        VStack(alignment: .leading, spacing: 3) {
            Text(evaluation.decision.summary).font(.subheadline.weight(.semibold))
            Text(
                "\(evaluation.policyMode.label) · \(evaluation.outcome) · " +
                evaluation.createdAt.formatted(date: .abbreviated, time: .shortened)
            )
                .font(.caption2)
                .foregroundStyle(PlantyColor.secondaryText)
            ForEach(evaluation.enforced, id: \.self) { Text($0).font(.caption) }
        }
    }
}

private struct PolicyReferenceScreen: View {
    @Environment(AppSession.self) private var session

    var body: some View {
        List {
            if let reference = session.policies.reference {
                Section("Contract") {
                    LabeledContent("Input", value: reference.inputVersion)
                    LabeledContent("Entrypoint", value: reference.entrypoint)
                }
                ForEach(reference.sections) { section in
                    Section(section.title) {
                        ForEach(section.fields) { field in ReferenceFieldRow(field: field) }
                    }
                }
                Section("Output") {
                    ForEach(reference.output) { field in ReferenceFieldRow(field: field) }
                }
                Section("Safety rules") {
                    ForEach(reference.safety, id: \.self) { rule in
                        Label(rule, systemImage: "lock.shield.fill")
                    }
                }
                Section("Complete example") {
                    Text(reference.example)
                        .font(.system(.caption, design: .monospaced))
                        .textSelection(.enabled)
                }
            } else if let error = session.policies.error {
                SheetErrorRow(headline: "The reference could not be loaded.", error: error)
            } else {
                HStack { ProgressView(); Text("Loading contract…") }
            }
        }
        .scrollContentBackground(.hidden)
        .plantyPage()
        .navigationTitle("Policy reference")
        .task { if session.policies.reference == nil { await session.policies.load() } }
    }
}

private struct ReferenceFieldRow: View {
    let field: PolicyReferenceField

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(field.path).font(.system(.caption, design: .monospaced).weight(.semibold))
            Text(field.type).font(.caption2).foregroundStyle(PlantyColor.purple)
            Text(field.description).font(.caption).foregroundStyle(PlantyColor.secondaryText)
        }
        .textSelection(.enabled)
    }
}

private extension PolicyMode {
    var label: String { self == .enforce ? "Enforce" : "Advisory" }
}

private extension PolicySignalKind {
    var label: String { rawValue.replacingOccurrences(of: "_", with: " ").capitalized }
    var symbol: String {
        switch self {
        case .needsWatered: "drop.fill"
        case .needsMisted: "humidity.fill"
        case .moveInside: "house.fill"
        case .moveOutside: "sun.max.fill"
        case .incident: "exclamationmark.triangle.fill"
        case .health: "heart.fill"
        case .airflow: "fan.fill"
        case .unknown: "questionmark.circle"
        }
    }
}

private extension PolicySeverity {
    var color: Color {
        switch self {
        case .info: PlantyColor.cyan
        case .warning: PlantyColor.orange
        case .critical: PlantyColor.red
        case .unknown: PlantyColor.secondaryText
        }
    }
}
