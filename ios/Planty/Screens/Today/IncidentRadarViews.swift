import SwiftUI

struct IncidentRadarSection: View {
    @Environment(AppSession.self) private var session

    var body: some View {
        if !session.incidents.unresolved.isEmpty {
            VStack(alignment: .leading, spacing: 12) {
                SectionHeading("Garden Incident Radar", detail: "Shared timing is a lead to investigate. Correlation is not causation.")
                ForEach(session.incidents.unresolved) { incident in
                    NavigationLink { IncidentDetailScreen(incidentID: incident.id) } label: {
                        IncidentCard(incident: incident)
                    }
                    .buttonStyle(.plain)
                }
            }
        } else if let error = session.incidents.error {
            Label(error.errorDescription ?? "Incident Radar could not load.", systemImage: "exclamationmark.triangle")
                .font(.caption).foregroundStyle(PlantyColor.orange)
        }
    }
}

private struct IncidentCard: View {
    let incident: GardenIncident

    var body: some View {
        VStack(alignment: .leading, spacing: 9) {
            HStack(alignment: .firstTextBaseline) {
                Label("Possible shared incident", systemImage: "waveform.path.ecg.rectangle")
                    .font(.headline).foregroundStyle(PlantyColor.orange)
                Spacer(minLength: 8)
                Text(incident.status.label).font(.caption.weight(.semibold))
            }
            Text(incident.summary).font(.subheadline)
            Text("Suspected factor: \(incident.suspectedFactorType.label) · \(incident.suspectedFactorRef)")
                .font(.caption).foregroundStyle(PlantyColor.secondaryText)
            Divider().overlay(PlantyColor.quietDecoration)
            ForEach(incident.plants) { member in
                HStack(alignment: .firstTextBaseline) {
                    Text(member.plant.commonName).font(.subheadline.weight(.semibold))
                    Spacer()
                    Text(member.action.shortLabel).font(.subheadline).foregroundStyle(member.action == .urgent ? PlantyColor.red : PlantyColor.orange)
                }
            }
            Text("Keep handling each urgent plant action individually while investigating the possible connection.")
                .font(.caption).foregroundStyle(PlantyColor.secondaryText)
        }
        .plantyCard(border: PlantyColor.orange.opacity(0.3), padding: 14)
        .accessibilityElement(children: .combine)
    }
}

private struct IncidentDetailScreen: View {
    let incidentID: UUID
    @Environment(AppSession.self) private var session
    @State private var resolves = false
    @State private var failure: PlantyError?

    private var incident: GardenIncident? { session.incidents.incidents.first { $0.id == incidentID } }

    var body: some View {
        List {
            if let incident {
                Section {
                    Text("Correlation is not causation. This groups inspectable signals into a hypothesis; it does not replace any plant's individual urgent action.")
                        .font(.subheadline).foregroundStyle(PlantyColor.secondaryText)
                }
                Section("Suspected connection") {
                    Text(incident.summary)
                    LabeledContent("Factor", value: incident.suspectedFactorType.label)
                    LabeledContent("Reference", value: incident.suspectedFactorRef)
                    LabeledContent("Confidence", value: incident.confidence.formatted(.percent.precision(.fractionLength(0))))
                    Text(incident.evidence.note).font(.caption).foregroundStyle(PlantyColor.secondaryText)
                }
                Section("Affected plants — keep actions separate") {
                    ForEach(incident.plants) { member in
                        VStack(alignment: .leading, spacing: 4) {
                            Text(member.plant.commonName).font(.headline)
                            Label(member.action.shortLabel, systemImage: member.action == .urgent ? "exclamationmark.triangle.fill" : "hand.raised.fill")
                                .foregroundStyle(member.action == .urgent ? PlantyColor.red : PlantyColor.orange)
                            Text("Last signal \(member.lastSeenAt.formatted(date: .abbreviated, time: .shortened))")
                                .font(.caption).foregroundStyle(PlantyColor.secondaryText)
                        }
                    }
                }
                Section("Evidence ledger") {
                    evidenceCount("Verdicts", incident.evidence.verdictIDs.count)
                    evidenceCount("Observations", incident.evidence.observationIDs.count)
                    evidenceCount("Sensors", incident.evidence.sensorLinkIDs.count)
                    evidenceCount("Actuator events", incident.evidence.actuatorEventIDs.count)
                }
                if let failure { Section { SheetErrorRow(headline: "The incident was not changed.", error: failure) } }
                if incident.status == .open {
                    Section { Button("Acknowledge investigation") { Task { failure = await session.incidents.acknowledge(incident) } } }
                }
                if incident.status != .resolved {
                    Section { Button("Resolve with evidence…") { resolves = true } }
                }
                if incident.status == .resolved {
                    Section("Resolution") {
                        Text(incident.resolution?.label ?? "Resolved")
                        if let conclusion = incident.conclusion?.nilIfBlank { Text(conclusion) }
                    }
                }
            }
        }
        .scrollContentBackground(.hidden).plantyPage()
        .navigationTitle("Incident detail").navigationBarTitleDisplayMode(.inline)
        .sheet(isPresented: $resolves) { if let incident { IncidentResolutionSheet(incident: incident) } }
    }

    private func evidenceCount(_ label: String, _ count: Int) -> some View {
        LabeledContent(label, value: count.formatted())
    }
}

private struct IncidentResolutionSheet: View {
    let incident: GardenIncident
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss
    @State private var outcome: IncidentResolution = .inconclusive
    @State private var conclusion = ""
    @State private var failure: PlantyError?

    private var choices: [IncidentResolution] { [.confirmedCommonCause, .unrelated, .contained, .inconclusive] }

    var body: some View {
        NavigationStack {
            Form {
                Text("Choose what the evidence supports. Similar timing alone does not confirm a shared cause.")
                    .font(.subheadline).foregroundStyle(PlantyColor.secondaryText)
                Picker("Resolution", selection: $outcome) { ForEach(choices, id: \.self) { Text($0.label).tag($0) } }
                TextField("What did the evidence show?", text: $conclusion, axis: .vertical).lineLimit(3...7)
                if let failure { SheetErrorRow(headline: "The incident was not resolved.", error: failure) }
            }
            .plantyPage().navigationTitle("Resolve incident").navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) { Button("Cancel") { dismiss() } }
                ToolbarItem(placement: .confirmationAction) { Button("Resolve") { Task { await submit() } }.disabled(conclusion.cleaned.isEmpty) }
            }
        }
    }

    private func submit() async {
        failure = await session.incidents.resolve(incident, outcome: outcome, conclusion: conclusion)
        if failure == nil { dismiss() }
    }
}

private extension IncidentStatus {
    var label: String { rawValue.capitalized }
}

private extension IncidentFactor {
    var label: String { rawValue.replacingOccurrences(of: "_", with: " ").capitalized }
}

private extension IncidentResolution {
    var label: String { rawValue.replacingOccurrences(of: "_", with: " ").capitalized }
}
