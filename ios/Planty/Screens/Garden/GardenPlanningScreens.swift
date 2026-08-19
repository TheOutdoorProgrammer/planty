import SwiftUI

struct AwayPlannerScreen: View {
    @Bindable var store: GardenStore

    @State private var startsAt = Calendar.current.date(byAdding: .day, value: 1, to: Date()) ?? Date()
    @State private var endsAt = Calendar.current.date(byAdding: .day, value: 4, to: Date()) ?? Date()
    @State private var backupContact = ""
    @State private var backupNotify = ""
    @State private var note = ""
    @State private var saving = false
    @State private var failure: PlantyError?

    var body: some View {
        Form {
            if let saved = store.plannedAway {
                Section {
                    Label("Coverage planned", systemImage: "checkmark.circle.fill")
                        .font(.headline)
                        .foregroundStyle(PlantyColor.green)
                    Text("\(saved.startsAt.formatted(date: .abbreviated, time: .shortened)) to \(saved.endsAt.formatted(date: .abbreviated, time: .shortened))")
                        .foregroundStyle(PlantyColor.secondaryText)
                }
                .listRowBackground(PlantyColor.surface)
            }

            if let failure {
                Section { SheetErrorRow(headline: "The trip was not saved.", error: failure) }
            }

            Section("When") {
                DatePicker("Leaving", selection: $startsAt)
                DatePicker("Returning", selection: $endsAt)
            }
            .listRowBackground(PlantyColor.surface)

            Section {
                TextField("Person (optional)", text: $backupContact)
                    .textContentType(.name)
                TextField("Notification target (optional)", text: $backupNotify)
                    .textInputAutocapitalization(.never)
                    .autocorrectionDisabled()
                TextField("Anything they should know", text: $note, axis: .vertical)
                    .lineLimit(2...6)
            } header: {
                Text("Backup")
            } footer: {
                Text("Planty uses this window when it decides what needs doing before you leave and who should hear about it while you are gone.")
            }
            .listRowBackground(PlantyColor.surface)

            Section {
                Button {
                    Task { await save() }
                } label: {
                    if saving {
                        ProgressView().tint(PlantyColor.background)
                    } else {
                        Text("Plan coverage")
                    }
                }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
                .disabled(!datesAreValid || saving)
                .listRowInsets(EdgeInsets())
                .listRowBackground(Color.clear)
            }
        }
        .plantyPage()
        .navigationTitle("Time away")
    }

    private var datesAreValid: Bool { endsAt > startsAt }

    private func save() async {
        saving = true
        failure = await store.planAway(
            NewAwayPeriod(
                startsAt: startsAt,
                endsAt: endsAt,
                backupContact: backupContact.nilIfBlank,
                backupNotify: backupNotify.nilIfBlank,
                note: note.nilIfBlank
            )
        )
        saving = false
    }
}

struct ColdWatchScreen: View {
    @Bindable var store: GardenStore

    @State private var forecastLowF = 40.0
    @State private var checking = false
    @State private var failure: PlantyError?

    var body: some View {
        List {
            Section {
                VStack(alignment: .leading, spacing: 12) {
                    Eyebrow(text: "Forecast low", color: PlantyColor.purple)
                    HStack(alignment: .firstTextBaseline) {
                        Text(forecastLowF.formatted(.number.precision(.fractionLength(0))))
                            .font(.system(size: 48, weight: .bold, design: .rounded))
                            .foregroundStyle(PlantyColor.foreground)
                        Text("°F")
                            .font(.title3.weight(.semibold))
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                    Slider(value: $forecastLowF, in: 0...70, step: 1)
                        .tint(PlantyColor.purple)
                    Stepper("Adjust forecast", value: $forecastLowF, in: 0...70, step: 1)
                        .labelsHidden()
                }
                .padding(.vertical, 8)
            }
            .listRowBackground(PlantyColor.surface)

            if let failure {
                Section { SheetErrorRow(headline: "The cold watch did not load.", error: failure) }
            }

            Section {
                Button {
                    Task { await check() }
                } label: {
                    if checking {
                        ProgressView().tint(PlantyColor.background)
                    } else {
                        Text("Check the garden")
                    }
                }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.purple))
                .disabled(checking)
                .listRowInsets(EdgeInsets())
                .listRowBackground(Color.clear)
            }

            if let watch = store.coldWatch {
                Section(watch.plants.isEmpty ? "No plants need shelter" : "Bring these in") {
                    if watch.plants.isEmpty {
                        Label("Everything can stay where it is", systemImage: "checkmark.circle.fill")
                            .foregroundStyle(PlantyColor.green)
                            .listRowBackground(PlantyColor.surface)
                    } else {
                        ForEach(watch.plants) { plant in
                            VStack(alignment: .leading, spacing: 4) {
                                Text(plant.commonName)
                                    .font(.headline)
                                    .foregroundStyle(PlantyColor.foreground)
                                HStack {
                                    Text(plant.location)
                                    if let threshold = plant.minTempF {
                                        Text("· below \(threshold.formatted())°F")
                                    }
                                }
                                .font(.subheadline)
                                .foregroundStyle(PlantyColor.secondaryText)
                            }
                            .padding(.vertical, 4)
                            .listRowBackground(PlantyColor.surface)
                        }
                    }
                }
            }
        }
        .plantyPage()
        .navigationTitle("Cold watch")
    }

    private func check() async {
        checking = true
        failure = await store.checkCold(forecastLowF: forecastLowF)
        checking = false
    }
}

struct GardenHistoryScreen: View {
    @Bindable var store: GardenStore
    @State private var selection = GardenHistoryKind.harvests

    var body: some View {
        List {
            Section {
                Picker("History type", selection: $selection) {
                    ForEach(GardenHistoryKind.allCases) { kind in
                        Text(kind.label).tag(kind)
                    }
                }
                .pickerStyle(.segmented)
                .listRowBackground(PlantyColor.surface)
            }

            switch selection {
            case .harvests:
                harvests
            case .lessons:
                lessons
            }
        }
        .plantyPage()
        .navigationTitle("Garden history")
        .refreshable { await store.load() }
    }

    @ViewBuilder
    private var harvests: some View {
        if store.harvests.isEmpty {
            empty("No harvests yet", icon: "basket")
        } else {
            Section("\(store.harvests.count) harvests") {
                ForEach(store.harvests) { harvest in
                    VStack(alignment: .leading, spacing: 5) {
                        HStack {
                            Text(harvest.commonName ?? harvest.slug ?? "Plant")
                                .font(.headline)
                            Spacer()
                            Text("\(harvest.quantity.formatted()) \(harvest.unit)")
                                .font(.headline.monospacedDigit())
                                .foregroundStyle(PlantyColor.green)
                        }
                        Text(harvest.occurredAt.formatted(date: .abbreviated, time: .omitted))
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                        if let notes = harvest.notes, !notes.isEmpty {
                            Text(notes)
                                .font(.subheadline)
                                .foregroundStyle(PlantyColor.secondaryText)
                        }
                    }
                    .padding(.vertical, 5)
                    .listRowBackground(PlantyColor.surface)
                }
            }
        }
    }

    @ViewBuilder
    private var lessons: some View {
        if store.postmortems.isEmpty {
            empty("No lessons recorded", icon: "book.closed")
        } else {
            Section("\(store.postmortems.count) lessons") {
                ForEach(store.postmortems) { postmortem in
                    VStack(alignment: .leading, spacing: 6) {
                        Text(postmortem.commonName ?? postmortem.slug ?? "Plant")
                            .font(.headline)
                        Text(postmortem.likelyCause)
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(PlantyColor.orange)
                        Text(postmortem.lesson ?? postmortem.narrative)
                            .font(.subheadline)
                            .foregroundStyle(PlantyColor.secondaryText)
                        Text(postmortem.createdAt.formatted(date: .abbreviated, time: .omitted))
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                    }
                    .padding(.vertical, 5)
                    .listRowBackground(PlantyColor.surface)
                }
            }
        }
    }

    private func empty(_ title: String, icon: String) -> some View {
        ContentUnavailableView(title, systemImage: icon)
            .foregroundStyle(PlantyColor.secondaryText)
            .listRowBackground(Color.clear)
    }
}

private enum GardenHistoryKind: String, CaseIterable, Identifiable {
    case harvests
    case lessons

    var id: String { rawValue }

    var label: String {
        switch self {
        case .harvests: "Harvests"
        case .lessons: "Lessons"
        }
    }
}
