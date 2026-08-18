import Foundation

/// What Fledge answers when an app asks whether it is behind.
struct FledgeRelease: Decodable, Sendable, Equatable {
    let version: String
    let build: String
    let installPageURL: URL
    let updateAvailable: Bool
    var notes: [FledgeNote] = []

    enum CodingKeys: String, CodingKey {
        case version
        case build
        case installPageURL = "install_page_url"
        case updateAvailable = "update_available"
        case notes = "changelog"
    }

    /// "1.0 (4)", which is what Settings shows for the running build, so the
    /// two read the same way side by side.
    var label: String { "\(version) (\(build))" }
}

struct FledgeNote: Decodable, Sendable, Equatable, Identifiable {
    let version: String
    let build: String
    var notes: String?

    var id: String { "\(version)-\(build)" }
}

protocol FledgeUpdating: Sendable {
    /// Nil when there is nothing to say: no server configured, or the server
    /// could not be reached. An update check is never worth an error state.
    func check(runningBuild: String) async -> FledgeRelease?
}

/// Asks a Fledge server. Distribution is not the app's job, so a failure here
/// is silence rather than anything the user has to deal with.
struct FledgeUpdateService: FledgeUpdating {
    let server: URL?
    let bundleID: String
    let session: URLSession

    init(server: URL?, bundleID: String, session: URLSession = .plantyDefault) {
        self.server = server
        self.bundleID = bundleID
        self.session = session
    }

    func check(runningBuild: String) async -> FledgeRelease? {
        guard let server else { return nil }

        var components = URLComponents(
            url: server.appending(path: "/api/v1/apps/\(bundleID)/latest"),
            resolvingAgainstBaseURL: false
        )
        components?.queryItems = [URLQueryItem(name: "build", value: runningBuild)]
        guard let url = components?.url else { return nil }

        do {
            let (data, response) = try await session.data(from: url)
            guard let http = response as? HTTPURLResponse, http.statusCode == 200 else {
                return nil
            }
            return try JSONDecoder().decode(FledgeRelease.self, from: data)
        } catch {
            // An older server has no such route, and a phone off the network
            // has no server at all. Both are silence.
            return nil
        }
    }
}

extension ConfigurationKey {
    /// Fed by the FLEDGE_BASE_URL build setting, like the Planty URL beside it.
    /// Absent means the app was not built for over-the-air distribution.
    static let fledgeURLPlist = "FledgeBaseURL"
}

extension FledgeUpdateService {
    /// Built from what the archive baked in. Returns a service that answers nil
    /// when no server was configured, rather than an optional service.
    static func fromBundle(_ bundle: Bundle = .main) -> FledgeUpdateService {
        let raw = (bundle.object(forInfoDictionaryKey: ConfigurationKey.fledgeURLPlist) as? String)?
            .trimmingCharacters(in: .whitespacesAndNewlines)
        let identifier = bundle.bundleIdentifier ?? ""

        return FledgeUpdateService(
            server: (raw?.isEmpty == false) ? URL(string: raw!) : nil,
            bundleID: identifier
        )
    }

    static var runningBuild: String {
        (Bundle.main.infoDictionary?["CFBundleVersion"] as? String) ?? ""
    }
}
