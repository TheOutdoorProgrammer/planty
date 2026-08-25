import SwiftUI

struct AwayPlannerScreen: View {
    @Bindable var store: GardenStore

    @State private var editingID: UUID?
    @State private var pendingCancel: AwayPeriod?
    @State private var startsAt = Calendar.current.date(byAdding: .day, value: 1, to: Date()) ?? Date()
    @State private var endsAt = Calendar.current.date(byAdding: .day, value: 4, to: Date()) ?? Date()
    @State private var backupContact = ""
    @State private var backupNotify = ""
    @State private var note = ""
    @State private var saving = false
    @State private var failure: PlantyError?

    var body: some View {
        Form {
            if !store.awayPeriods.isEmpty {
                Section("Planned coverage") {
                    ForEach(store.awayPeriods) { period in
                        VStack(alignment: .leading, spacing: 8) {
                            HStack {
                                Label(
                                    period.Covers(Date()) ? "Active now" : "Upcoming",
                                    systemImage: period.Covers(Date()) ? "house.fill" : "calendar"
                                )
                                .font(.headline)
                                .foregroundStyle(period.Covers(Date()) ? PlantyColor.orange : PlantyColor.green)
                                Spacer()
                                if editingID == period.id {
                                    Text("Editing")
                                        .font(.caption.weight(.semibold))
                                        .foregroundStyle(PlantyColor.cyan)
                                }
                            }
                            Text("\(period.startsAt.formatted(date: .abbreviated, time: .shortened)) to \(period.endsAt.formatted(date: .abbreviated, time: .shortened))")
                                .foregroundStyle(PlantyColor.secondaryText)
                            if let contact = period.backupContact, !contact.isEmpty {
                                Label(contact, systemImage: "person.fill")
                                    .font(.subheadline)
                            }
                            if let note = period.note, !note.isEmpty {
                                Text(note)
                                    .font(.subheadline)
                                    .foregroundStyle(PlantyColor.secondaryText)
                            }
                            HStack {
                                Button("Edit") { edit(period) }
                                    .buttonStyle(.borderless)
                                Spacer()
                                Button("Cancel coverage", role: .destructive) {
                                    pendingCancel = period
                                }
                                .buttonStyle(.borderless)
                            }
                            .font(.subheadline.weight(.semibold))
                        }
                        .padding(.vertical, 5)
                    }
                }
                .listRowBackground(PlantyColor.surface)
            }

            if let failure {
                Section { SheetErrorRow(headline: "The coverage change failed.", error: failure) }
            }

            Section(editingID == nil ? "New coverage" : "Edit coverage") {
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
                Text("Planty uses this window when it decides what needs doing before you leave and who should hear about it while you are gone. Coverage windows cannot overlap.")
            }
            .listRowBackground(PlantyColor.surface)

            Section {
                Button {
                    Task { await save() }
                } label: {
                    if saving {
                        ProgressView().tint(PlantyColor.background)
                    } else {
                        Text(editingID == nil ? "Plan coverage" : "Save changes")
                    }
                }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
                .disabled(!datesAreValid || saving)
                .listRowInsets(EdgeInsets())
                .listRowBackground(Color.clear)

                if editingID != nil {
                    Button("Stop editing") { resetForm() }
                        .foregroundStyle(PlantyColor.secondaryText)
                }
            }
        }
        .plantyPage()
        .navigationTitle("Time away")
        .confirmationDialog(
            "Cancel this coverage?",
            isPresented: Binding(
                get: { pendingCancel != nil },
                set: { if !$0 { pendingCancel = nil } }
            ),
            titleVisibility: .visible,
            presenting: pendingCancel
        ) { period in
            Button("Cancel coverage", role: .destructive) {
                Task { await cancel(period) }
            }
            Button("Keep coverage", role: .cancel) {}
        } message: { period in
            Text("\(period.startsAt.formatted(date: .abbreviated, time: .omitted)) to \(period.endsAt.formatted(date: .abbreviated, time: .omitted)) will be removed.")
        }
    }

    private var datesAreValid: Bool { endsAt > startsAt }

    private func edit(_ period: AwayPeriod) {
        editingID = period.id
        startsAt = period.startsAt
        endsAt = period.endsAt
        backupContact = period.backupContact ?? ""
        backupNotify = period.backupNotify ?? ""
        note = period.note ?? ""
        failure = nil
    }

    private func resetForm() {
        editingID = nil
        startsAt = Calendar.current.date(byAdding: .day, value: 1, to: Date()) ?? Date()
        endsAt = Calendar.current.date(byAdding: .day, value: 4, to: Date()) ?? Date()
        backupContact = ""
        backupNotify = ""
        note = ""
    }

    private func draft() -> NewAwayPeriod {
        NewAwayPeriod(
            startsAt: startsAt,
            endsAt: endsAt,
            backupContact: backupContact.nilIfBlank,
            backupNotify: backupNotify.nilIfBlank,
            note: note.nilIfBlank
        )
    }

    private func save() async {
        saving = true
        if let editingID {
            failure = await store.updateAway(id: editingID, draft: draft())
        } else {
            failure = await store.planAway(draft())
        }
        saving = false
        if failure == nil { resetForm() }
    }

    private func cancel(_ period: AwayPeriod) async {
        pendingCancel = nil
        saving = true
        failure = await store.cancelAway(id: period.id)
        saving = false
        if failure == nil, editingID == period.id { resetForm() }
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
    @State private var editingHarvest: Harvest?
    @State private var deletingHarvest: Harvest?
    @State private var actionError: PlantyError?

    var body: some View {
        List {
            if let actionError {
                Section {
                    SheetErrorRow(headline: "The harvest was not changed.", error: actionError)
                }
            }
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
        .sheet(item: $editingHarvest) { harvest in
            HarvestSheet(
                plantName: harvest.commonName ?? harvest.slug ?? "Plant",
                harvest: harvest
            ) { quantity, unit, notes in
                await store.updateHarvest(harvest, quantity: quantity, unit: unit, notes: notes)
            }
        }
        .confirmationDialog(
            "Delete this harvest?",
            isPresented: .init(get: { deletingHarvest != nil }, set: { if !$0 { deletingHarvest = nil } }),
            titleVisibility: .visible
        ) {
            Button("Delete harvest", role: .destructive) {
                guard let harvest = deletingHarvest else { return }
                deletingHarvest = nil
                Task { actionError = await store.deleteHarvest(harvest) }
            }
            Button("Keep it", role: .cancel) { deletingHarvest = nil }
        } message: {
            Text("This changes the seasonal totals and cannot be undone.")
        }
    }

    @ViewBuilder
    private var harvests: some View {
        if store.harvests.isEmpty {
            empty("No harvests yet", icon: "basket")
        } else {
            if !store.harvestSummary.isEmpty {
                Section("Season totals") {
                    ForEach(store.harvestSummary) { total in
                        LabeledContent {
                            Text("\(total.quantity.formatted()) \(total.unit)")
                                .font(.headline.monospacedDigit())
                                .foregroundStyle(PlantyColor.green)
                        } label: {
                            Text("\(total.commonName) · \(total.season.capitalized) \(total.year.formatted(.number.grouping(.never)))")
                        }
                        .listRowBackground(PlantyColor.surface)
                    }
                }
            }
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
                            Menu {
                                Button { editingHarvest = harvest } label: {
                                    Label("Edit", systemImage: "pencil")
                                }
                                Button(role: .destructive) { deletingHarvest = harvest } label: {
                                    Label("Delete", systemImage: "trash")
                                }
                            } label: {
                                Image(systemName: "ellipsis.circle")
                                    .frame(width: 44, height: 44)
                            }
                            .accessibilityLabel("Actions for this harvest")
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
