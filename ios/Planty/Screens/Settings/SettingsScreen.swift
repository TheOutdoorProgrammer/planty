import SwiftUI

/// Behind the profile button, never a daily dashboard card: connection, data
/// freshness and sensor state live here so they stop competing with the task.
struct SettingsScreen: View {
    @Environment(AppSession.self) private var session
    @Environment(\.dismiss) private var dismiss

    @State private var baseURL = ""
    @State private var token = ""
    @State private var probe: ProbeResult = .idle

    var body: some View {
        NavigationStack {
            Form {
                connectionSection
                householdSection
                freshnessSection
                sensorsSection
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

    private var sensorsSection: some View {
        Section {
            NavigationLink("Sensor connections") {
                SensorListScreen(api: session.api)
            }
        } footer: {
            Text("An uncalibrated probe reports, but never drives a decision.")
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

/// An uncalibrated probe reports and is ignored, which looks identical to a
/// working one from here, so the state is on every row and tapping fixes it.
struct SensorListScreen: View {
    let api: any PlantyAPI

    @State private var links: [SensorLink] = []
    @State private var hasLoaded = false
    @State private var error: PlantyError?
    @State private var calibrating: SensorLink?
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
            } else if links.isEmpty && error == nil {
                emptyState
            }
            ForEach(links) { link in
                Button {
                    calibrating = link
                } label: {
                    row(for: link)
                }
                .buttonStyle(.plain)
                .listRowBackground(PlantyColor.surface)
                .accessibilityHint("Opens calibration.")
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
        .sheet(item: $calibrating) { link in
            CalibrateSensorSheet(link: link) { calibration in
                await apply(calibration, to: link)
            }
        }
        .sheet(isPresented: $isLinking) {
            LinkSensorSheet(api: api) { saved in
                links.append(saved)
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
            Label(
                link.isCalibrated ? "Calibrated" : "Not calibrated, so it cannot drive watering",
                systemImage: link.isCalibrated ? "checkmark.seal.fill" : "exclamationmark.triangle.fill"
            )
            .font(.caption)
            .foregroundStyle(link.isCalibrated ? PlantyColor.green : PlantyColor.yellow)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .contentShape(Rectangle())
    }

    private func load() async {
        do {
            links = try await api.sensors()
            error = nil
        } catch {
            guard !PlantyError.isCancellation(error) else { return }
            self.error = PlantyError.from(error)
        }
        hasLoaded = true
    }

    /// Returns the failure so the calibration sheet can stay open with the
    /// typed baselines instead of dismissing into a list-level error.
    private func apply(_ calibration: SensorCalibration, to link: SensorLink) async -> PlantyError? {
        do {
            let saved = try await api.calibrate(sensorID: link.id, to: calibration)
            if let index = links.firstIndex(where: { $0.id == saved.id }) {
                links[index] = saved
            }
            return nil
        } catch {
            return PlantyError.from(error)
        }
    }
}
