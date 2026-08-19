import Foundation
import Observation

enum AppTab: String, Hashable, Sendable {
    case today
    case snap
    case plants
}

/// A plant carried into Snap, so "I'm here" opens capture already knowing what
/// it is looking at and what was recommended.
struct SnapContext: Sendable, Equatable {
    var plant: Plant
    var recommendedAction: VerdictAction?
    var verdictID: UUID?
}

/// Owns the configuration, the client built from it, and the long-lived stores.
@Observable
@MainActor
final class AppSession {
    var selectedTab: AppTab = .today
    var snapContext: SnapContext?
    var isShowingSettings = false

    private(set) var configuration: PlantyConfiguration
    private(set) var api: any PlantyAPI

    let today: TodayStore
    let library: PlantsStore
    let capture: CaptureStore
    private(set) var identification: IdentificationStore
    let updates: UpdateStore

    private let defaults: UserDefaults
    private let tokens: any TokenStoring

    init(
        defaults: UserDefaults = .standard,
        tokens: any TokenStoring = KeychainTokenStore(),
        api: (any PlantyAPI)? = nil,
    ) {
        self.defaults = defaults
        self.tokens = tokens

        let resolved = PlantyConfiguration.resolve(defaults: defaults, tokens: tokens)
        configuration = resolved
        let client = api ?? PlantyClient(configuration: resolved)
        self.api = client

        today = TodayStore(api: client, isConfigured: resolved.isConfigured)
        library = PlantsStore(api: client, isConfigured: resolved.isConfigured)
        capture = CaptureStore(api: client)
        identification = IdentificationStore(
            pipeline: Self.pipeline(client: client, configured: resolved.isConfigured)
        )
        // Distribution is not the service's business, so this reads its own
        // server out of the build rather than the Planty configuration.
        updates = UpdateStore(service: FledgeUpdateService.fromBundle())

        #if DEBUG
        // Lets a screenshot run open straight onto a tab. Debug only.
        if let name = ProcessInfo.processInfo.environment["PLANTY_START_TAB"],
           let tab = AppTab(rawValue: name) {
            selectedTab = tab
        }
        #endif
    }

    /// Rebuilds the client and re-asks, because a new base URL invalidates
    /// every cached answer the old one gave.
    func updateConfiguration(baseURL: String, token: String) {
        let trimmedURL = baseURL.trimmingCharacters(in: .whitespacesAndNewlines)
        defaults.set(trimmedURL.isEmpty ? nil : trimmedURL, forKey: ConfigurationKey.baseURLDefaults)

        let trimmedToken = token.trimmingCharacters(in: .whitespacesAndNewlines)
        tokens.setToken(
            trimmedToken.isEmpty ? nil : trimmedToken,
            for: ConfigurationKey.tokenKeychainAccount
        )

        configuration = PlantyConfiguration.resolve(defaults: defaults, tokens: tokens)
        let client = PlantyClient(configuration: configuration)
        api = client

        today.replace(api: client, isConfigured: configuration.isConfigured)
        library.replace(api: client, isConfigured: configuration.isConfigured)
        capture.replace(api: client)
        identification = IdentificationStore(
            pipeline: Self.pipeline(client: client, configured: configuration.isConfigured)
        )
    }

    /// The species half needs a service; the Vision half never does, so an
    /// unconfigured app still gates and still says what it can see.
    private static func pipeline(client: any PlantyAPI, configured: Bool) -> IdentificationPipeline {
        IdentificationPipeline(
            intake: PhotoLibraryIntake(),
            analyzer: VisionImageAnalyzer(),
            identifier: configured
                ? RemotePlantIdentifier(api: client)
                : UnavailableIdentifier(),
            cache: FileIdentificationCache()
        )
    }

    func storyStore(for plant: Plant) -> PlantStoryStore {
        PlantStoryStore(api: api, plant: plant)
    }

    /// `asking` is sent the moment the screen opens, for entry points that
    /// already know the question and would only make the user retype it.
    func consultStore(for plant: Plant, asking question: String? = nil) -> ConsultStore {
        ConsultStore(api: api, plant: plant, pending: question)
    }

    /// A chat about a photograph. With a plant it joins that plant's story;
    /// with none it creates nothing and touches no timeline.
    func photoConsultStore(plant: Plant?, photo: Data?) -> ConsultStore {
        ConsultStore(
            api: api,
            plant: plant,
            attachment: photo,
            pending: plant == nil ? nil : "Here is a photo I just took. What do you see?"
        )
    }

    func remindersStore(for plant: Plant) -> RemindersStore {
        RemindersStore(api: api, plant: plant)
    }

    func notesStore(for plant: Plant) -> NotesStore {
        NotesStore(api: api, slug: plant.slug)
    }

    /// Sends the user to Snap with the plant locked in, which is the whole of
    /// what "I'm here" does.
    func beginCapture(for entry: DigestEntry) {
        snapContext = SnapContext(
            plant: entry.plant,
            recommendedAction: entry.verdict.action,
            verdictID: entry.verdict.id
        )
        selectedTab = .snap
    }

    func beginCapture(for plant: Plant) {
        snapContext = SnapContext(plant: plant, recommendedAction: nil, verdictID: nil)
        selectedTab = .snap
    }
}
