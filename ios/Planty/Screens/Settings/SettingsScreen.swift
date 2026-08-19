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

    private func saveAndTest() async {
        session.updateConfiguration(baseURL: baseURL, token: token)
        probe = .checking
        do {
            try await session.api.health()
            probe = .healthy
            await session.today.load()
            await session.library.load()
        } catch {
            probe = .failed(
                PlantyError.from(error).errorDescription ?? "The service did not answer."
            )
        }
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
    @State private var error: PlantyError?
    @State private var calibrating: SensorLink?

    var body: some View {
        List {
            if let error {
                Text(error.errorDescription ?? "Could not load sensors.")
                    .foregroundStyle(PlantyColor.orange)
            }
            ForEach(links) { link in
                Button {
                    calibrating = link
                } label: {
                    row(for: link)
                }
                .buttonStyle(.plain)
                .listRowBackground(PlantyColor.surface)
            }
        }
        .scrollContentBackground(.hidden)
        .plantyPage()
        .navigationTitle("Sensors")
        .navigationBarTitleDisplayMode(.inline)
        .sheet(item: $calibrating) { link in
            CalibrateSensorSheet(link: link) { calibration in
                calibrating = nil
                Task { await apply(calibration, to: link) }
            }
        }
        .task { await load() }
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
            self.error = PlantyError.from(error)
        }
    }

    private func apply(_ calibration: SensorCalibration, to link: SensorLink) async {
        do {
            let saved = try await api.calibrate(sensorID: link.id, to: calibration)
            if let index = links.firstIndex(where: { $0.id == saved.id }) {
                links[index] = saved
            }
            error = nil
        } catch {
            self.error = PlantyError.from(error)
        }
    }
}
