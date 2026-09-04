import SwiftUI

/// Behind the profile button, never a daily dashboard card: connection, data
/// freshness and sensor state live here so they stop competing with the task.
struct SettingsScreen: View {
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss

    @State private var baseURL = ""
    @State private var token = ""
    @State private var probe: ProbeResult = .idle
    @State private var push = PushRegistrationCenter.shared

    var body: some View {
        NavigationStack {
            Form {
                connectionSection
                notificationSection
                householdSection
                freshnessSection
                scheduledJobsSection
                modelsSection
                promptInstructionsSection
                policiesSection
                sensorsSection
                actuatorsSection
                aboutSection
            }
            .scrollContentBackground(.hidden)
            .plantyPage()
            .navigationTitle("Settings")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
            .task { load() }
        }
    }

    private var notificationSection: some View {
        Section {
            LabeledContent("Permission", value: permissionLabel)
            LabeledContent("APNs registration", value: progressLabel(push.apnsRegistration))
            LabeledContent("Token upload", value: progressLabel(push.tokenUpload))
            LabeledContent("App environment", value: push.environment)
            LabeledContent("Server environment", value: push.serverStatus?.environment ?? "unknown")

            if let accepted = acceptedAt(push.tokenUpload) {
                LabeledContent("Last server acceptance") {
                    Text(RelativeAge.dayAndTime(accepted, now: Date()))
                }
            }
            if let error = push.lastRegistrationError {
                LabeledContent("Last APNs error") {
                    Text(error).foregroundStyle(PlantyColor.red)
                }
            }
            if case .failed(let error) = push.testDelivery {
                Text(error).foregroundStyle(PlantyColor.red)
            }
            if case .accepted = push.testDelivery {
                Label("APNs accepted the test notification.", systemImage: "checkmark.circle.fill")
                    .foregroundStyle(PlantyColor.green)
            }

            Button(progressLabel(push.testDelivery) == "testing" ? "Sending test…" : "Send test notification") {
                Task { await push.testNotification() }
            }
            .disabled(push.testDelivery == .pending)

            Button("Retry notification setup") {
                Task { await push.recover() }
            }
        } header: {
            Text("Notifications")
        } footer: {
            Text(
                "Permission, Apple registration, token upload, and APNs delivery are separate checks. " +
                "The service connection test proves none of them."
            )
        }
        .task { await push.synchronize() }
    }

    private var permissionLabel: String {
        switch push.permission {
        case .authorized: "allowed"
        case .provisional: "provisional"
        case .ephemeral: "temporary"
        case .denied: "denied"
        case .notDetermined: "not requested"
        @unknown default: "unknown"
        }
    }

    private func progressLabel(_ progress: PushProgress) -> String {
        switch progress {
        case .idle: "not yet"
        case .pending: "testing"
        case .accepted: "accepted"
        case .failed: "failed"
        }
    }

    private func acceptedAt(_ progress: PushProgress) -> Date? {
        guard case .accepted(let date) = progress else { return nil }
        return date
    }

    /// What is true of the place rather than of any one plant. Kept here
    /// rather than on a plant's page, because it belongs to all of them.
    private var householdSection: some View {
        Section {
            NavigationLink {
                NotesScreen(store: session.householdNotesStore())
            } label: {
                Label("About the house", systemImage: "house.fill")
            }
        } footer: {
            Text("Read before every answer about every plant.")
        }
    }

    private var connectionSection: some View {
        Section {
            TextField("https://planty.example", text: $baseURL)
                .textContentType(.URL)
                .keyboardType(.URL)
                .textInputAutocapitalization(.never)
                .autocorrectionDisabled()
            SecureField("Bearer token", text: $token)

            Button(probe == .checking ? "Testing…" : "Test and save") {
                Task { await testAndSave() }
            }
            .disabled(probe == .checking)
            probeRow
        } header: {
            Text("Planty service")
        } footer: {
            Text("""
                The build can supply these with PLANTY_BASE_URL and \
                PLANTY_API_TOKEN. What you type here wins. The token is kept in \
                the Keychain.
                """)
        }
    }

    @ViewBuilder
    private var probeRow: some View {
        switch probe {
        case .idle:
            EmptyView()
        case .checking:
            Label("Asking the service…", systemImage: "ellipsis.circle")
                .foregroundStyle(PlantyColor.secondaryText)
        case .healthy:
            Label("The service answered.", systemImage: "checkmark.circle.fill")
                .foregroundStyle(PlantyColor.green)
        case .failed(let message):
            Label(message, systemImage: "xmark.circle.fill")
                .foregroundStyle(PlantyColor.red)
        }
    }

    /// The same honesty as Today: how old the evidence is, said plainly.
    private var freshnessSection: some View {
        Section("Data freshness") {
            if let digest = session.today.digest {
                LabeledContent("Last digest", value: RelativeAge.dayAndTime(digest.date, now: Date()))
                LabeledContent("Plants checked", value: "\(digest.checked)")
                if let staleSince = digest.staleSince {
                    LabeledContent("Service says stale since") {
                        Text(RelativeAge.dayAndTime(staleSince, now: Date()))
                            .foregroundStyle(PlantyColor.yellow)
                    }
                }
            } else {
                Text("No digest has loaded yet.")
                    .foregroundStyle(PlantyColor.secondaryText)
            }
        }
    }

    private var scheduledJobsSection: some View {
        Section {
            NavigationLink {
                ScheduledJobsScreen()
            } label: {
                Label("Run scheduled jobs", systemImage: "play.circle.fill")
            }
        } footer: {
            Text("Start the same bounded Kubernetes jobs Planty normally runs on its schedule.")
        }
    }

    /// One row per job. Each offers only the models the server says can do that
    /// job, so a model that cannot read a photograph is absent from the jobs
    /// that show one rather than merely discouraged.
    private var modelsSection: some View {
        Section {
            if let error = session.models.error {
                Text(error.errorDescription ?? "The models could not be loaded.")
                    .foregroundStyle(PlantyColor.secondaryText)
            } else if !session.models.hasLoaded {
                Text("Loading…").foregroundStyle(PlantyColor.secondaryText)
            } else {
                ForEach(AIJob.allCases.filter { $0 != .unknown }, id: \.self) { job in
                    let assignment = session.models.assignment(for: job)
                    ModelPickerField(
                        job: job,
                        choices: session.models.choices(for: job),
                        current: session.models.model(ref: assignment?.ref),
                        isDefault: assignment?.isDefault ?? true,
                        onChoose: { await session.models.assign($0, to: job) },
                        onUseDefault: { await session.models.useDefault(for: job) }
                    )
                }
            }
        } header: {
            Text("Which model answers what")
        } footer: {
            Text("Listed most capable first. A job only offers models that can actually do it.")
        }
        .task { await session.models.loadIfNeeded() }
    }

    private var sensorsSection: some View {
        Section {
            NavigationLink("Sensor connections") {
                SensorListScreen(api: session.api)
            }
        } footer: {
            Text("An uncalibrated soil probe reports, but never drives a decision.")
        }
    }

    private var promptInstructionsSection: some View {
        Section {
            NavigationLink {
                PromptInstructionSettingsScreen()
            } label: {
                Label("Prompt overlays", systemImage: "text.badge.plus")
            }
        } footer: {
            Text("Editable additions only. Code-owned safety, schema, tools, and authority remain in the base prompt.")
        }
    }

    private var policiesSection: some View {
        Section {
            NavigationLink {
                PolicySettingsScreen()
            } label: {
                Label("OPA policy rules", systemImage: "checkmark.shield.fill")
            }
        } footer: {
            Text(
                "Write and preview deterministic rules for care signals, agent guidance, " +
                "notifications, health, and opted-in fans."
            )
        }
    }

    private var actuatorsSection: some View {
        Section {
            NavigationLink {
                ActuatorSettingsScreen()
            } label: {
                Label("Fans, switches & lights", systemImage: "fan.fill")
            }
        } footer: {
            Text(
                "Register an exact Home Assistant entity before Planty can control it. " +
                "Grow lights can use Planty-owned daily schedules."
            )
        }
    }

    private var aboutSection: some View {
        Section("About") {
            // Without this there is no way to tell which build is on the phone,
            // which turned a stale install into an hour of chasing a fixed bug.
            LabeledContent("Version", value: Self.buildVersion)
            LabeledContent("Mascot", value: "A lavender seal, overwatering")
        }
    }

    private static var buildVersion: String {
        let info = Bundle.main.infoDictionary
        let short = info?["CFBundleShortVersionString"] as? String ?? "?"
        let build = info?["CFBundleVersion"] as? String ?? "?"
        return "\(short) (\(build))"
    }

    private func load() {
        baseURL = session.configuration.baseURL?.absoluteString ?? ""
        token = session.configuration.token ?? ""
    }

    /// Probes the typed configuration before anything persists: a typo'd URL
    /// must never replace a working one. Clearing the URL is an explicit
    /// unconfigure and skips the probe, because there is nothing left to ask.
    private func testAndSave() async {
        let typedURL = baseURL.trimmingCharacters(in: .whitespacesAndNewlines)
        let typedToken = token.trimmingCharacters(in: .whitespacesAndNewlines)

        guard !typedURL.isEmpty else {
            session.updateConfiguration(baseURL: baseURL, token: token)
            probe = .idle
            return
        }
        guard let url = URL(string: typedURL), url.scheme != nil else {
            probe = .failed("That is not a URL Planty can call. Nothing was saved.")
            return
        }

        probe = .checking
        let candidate = PlantyConfiguration(
            baseURL: url,
            token: typedToken.isEmpty ? nil : typedToken
        )
        do {
            try await PlantyClient(configuration: candidate).health()
        } catch {
            let reason = PlantyError.from(error).errorDescription ?? "The service did not answer."
            probe = .failed("\(reason) Nothing was saved; the working configuration still stands.")
            return
        }

        session.updateConfiguration(baseURL: baseURL, token: token)
        probe = .healthy
        await session.today.load()
        await session.library.load()
    }
}

enum ProbeResult: Equatable {
    case idle
    case checking
    case healthy
    case failed(String)
}

struct SensorListScreen: View {
    let api: any PlantyAPI

    @State private var sensors: [SensorSeries] = []
    @State private var plants: [Plant] = []
    @State private var hasLoaded = false
    @State private var error: PlantyError?
    @State private var editing: SensorSeries?
    @State private var isLinking = false

    var body: some View {
        List {
            if let error {
                Text(error.errorDescription ?? "Could not load sensors.")
                    .foregroundStyle(PlantyColor.orange)
                    .listRowBackground(PlantyColor.surface)
            }
            if !hasLoaded {
                // Before the first answer, a blank list would claim "nothing
                // is linked", which is the one thing not yet known.
                HStack(spacing: 12) {
                    ProgressView()
                    Text("Asking which probes are linked…")
                        .foregroundStyle(PlantyColor.secondaryText)
                }
                .listRowBackground(PlantyColor.surface)
            } else if sensors.isEmpty && error == nil {
                emptyState
            }
            ForEach(sensors) { sensor in
                Button {
                    editing = sensor
                } label: {
                    row(for: sensor.link)
                }
                .buttonStyle(.plain)
                .listRowBackground(PlantyColor.surface)
                .accessibilityHint("Opens sensor settings.")
            }
        }
        .scrollContentBackground(.hidden)
        .plantyPage()
        .navigationTitle("Sensors")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .primaryAction) {
                Button {
                    isLinking = true
                } label: {
                    Label("Link a sensor", systemImage: "plus")
                }
            }
        }
        .sheet(item: $editing) { sensor in
            SensorAssignmentSheet(api: api, sensor: sensor, onSaved: apply)
        }
        .sheet(isPresented: $isLinking) {
            LinkSensorSheet(api: api) { saved in
                let readings = sensors.first(where: { $0.id == saved.id })?.readings ?? []
                apply(SensorSeries(link: saved, readings: readings))
            }
        }
        .task { await load() }
    }

    private var emptyState: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text("No sensors are linked.")
                .font(.headline)
            Text("""
                Planty only trusts what it can measure. Link a Home Assistant \
                probe and its readings start backing the verdicts.
                """)
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
            Button("Link a sensor") { isLinking = true }
                .buttonStyle(SecondaryButtonStyle())
        }
        .listRowBackground(PlantyColor.surface)
    }

    private func row(for link: SensorLink) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(link.haEntityID)
                .font(.headline)
            Text(link.role.label)
                .font(.caption)
                .foregroundStyle(PlantyColor.secondaryText)
            Label(targetLabel(for: link), systemImage: link.plantID == nil ? "mappin" : "leaf.fill")
                .font(.caption)
                .foregroundStyle(PlantyColor.secondaryText)
            if link.role.requiresCalibration {
                Label(
                    link.isCalibrated ? "Calibrated" : "Not calibrated, so it cannot drive watering",
                    systemImage: link.isCalibrated ? "checkmark.seal.fill" : "exclamationmark.triangle.fill"
                )
                .font(.caption)
                .foregroundStyle(link.isCalibrated ? PlantyColor.green : PlantyColor.yellow)
            } else {
                Label("Uses the reported value directly", systemImage: "checkmark.seal.fill")
                    .font(.caption)
                    .foregroundStyle(PlantyColor.green)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .contentShape(Rectangle())
    }

    private func load() async {
        do {
            async let sensorTask = api.sensors()
            async let plantTask = api.plants(filter: .live)
            sensors = try await sensorTask
            plants = (try? await plantTask) ?? []
            error = nil
        } catch {
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
        hasLoaded = true
        #if DEBUG
        if let role = ProcessInfo.processInfo.environment["PLANTY_START_SENSOR_SETTINGS"] {
            editing = sensors.first { $0.link.role.rawValue == role } ?? sensors.first
        }
        #endif
    }

    private func targetLabel(for link: SensorLink) -> String {
        if let plantID = link.plantID {
            return plants.first(where: { $0.id == plantID })?.commonName ?? "Assigned plant"
        }
        return link.zone?.nilIfBlank ?? "Assigned place"
    }

    private func apply(_ saved: SensorSeries) {
        if let index = sensors.firstIndex(where: { $0.id == saved.id }) {
            sensors[index] = saved
        } else {
            sensors.append(saved)
        }
    }
}
