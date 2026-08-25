import Foundation
import Observation

enum AppTab: String, Hashable, Sendable {
    case today
    case snap
    case plants
    case garden
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
    var isShowingCareRound = false
    var pendingPlantSlug: String?

    private(set) var configuration: PlantyConfiguration
    private(set) var api: any PlantyAPI

    let today: TodayStore
    let library: PlantsStore
    let capture: CaptureStore
    let garden: GardenStore
    let choices: ManagedChoicesStore
    let models: ModelSettingsStore
    let promptInstructions: PromptInstructionStore
    let actuators: ActuatorStore
    let evidenceWorkflows: EvidenceWorkflowStore
    let incidents: IncidentStore
    let health: PlantHealthStore
    let images: ImageRepository
    private(set) var identification: IdentificationStore
    let updates: UpdateStore

    private let defaults: UserDefaults
    private let tokens: any TokenStoring
    private var apiGeneration = 0

    init(
        defaults: UserDefaults = .standard,
        tokens: any TokenStoring = KeychainTokenStore(),
        api: (any PlantyAPI)? = nil,
        images: ImageRepository? = nil,
    ) {
        self.defaults = defaults
        self.tokens = tokens
        let imageRepository = images ?? ImageRepository()
        self.images = imageRepository

        let resolved = PlantyConfiguration.resolve(defaults: defaults, tokens: tokens)
        configuration = resolved
        let client = api ?? PlantyClient(configuration: resolved, images: imageRepository)
        self.api = client

        today = TodayStore(api: client, isConfigured: resolved.isConfigured)
        library = PlantsStore(api: client, isConfigured: resolved.isConfigured)
        capture = CaptureStore(api: client)
        garden = GardenStore(api: client, isConfigured: resolved.isConfigured)
        choices = ManagedChoicesStore(api: client, isConfigured: resolved.isConfigured)
        models = ModelSettingsStore(api: client, isConfigured: resolved.isConfigured)
        promptInstructions = PromptInstructionStore(api: client, isConfigured: resolved.isConfigured)
        actuators = ActuatorStore(api: client, isConfigured: resolved.isConfigured)
        evidenceWorkflows = EvidenceWorkflowStore(api: client, isConfigured: resolved.isConfigured)
        incidents = IncidentStore(api: client, isConfigured: resolved.isConfigured)
        health = PlantHealthStore(api: client)
        identification = IdentificationStore(
            pipeline: Self.pipeline(
                client: client,
                configured: resolved.isConfigured,
                serviceID: resolved.baseURL?.absoluteString ?? "unconfigured"
            )
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
    /// every cached answer the old one gave. The generation also invalidates
    /// in-flight child-store requests that still hold the previous client.
    func updateConfiguration(baseURL: String, token: String) {
        let trimmedURL = baseURL.trimmingCharacters(in: .whitespacesAndNewlines)
        defaults.set(trimmedURL.isEmpty ? nil : trimmedURL, forKey: ConfigurationKey.baseURLDefaults)

        let trimmedToken = token.trimmingCharacters(in: .whitespacesAndNewlines)
        tokens.setToken(
            trimmedToken.isEmpty ? nil : trimmedToken,
            for: ConfigurationKey.tokenKeychainAccount
        )

        configuration = PlantyConfiguration.resolve(defaults: defaults, tokens: tokens)
        let client = PlantyClient(configuration: configuration, images: images)
        apiGeneration += 1
        api = client

        today.replace(api: client, isConfigured: configuration.isConfigured)
        library.replace(api: client, isConfigured: configuration.isConfigured)
        capture.replace(api: client)
        garden.replace(api: client, isConfigured: configuration.isConfigured)
        choices.replace(api: client, isConfigured: configuration.isConfigured)
        models.replace(api: client, isConfigured: configuration.isConfigured)
        promptInstructions.replace(api: client, isConfigured: configuration.isConfigured)
        actuators.replace(api: client, isConfigured: configuration.isConfigured)
        evidenceWorkflows.replace(api: client, isConfigured: configuration.isConfigured)
        incidents.replace(api: client, isConfigured: configuration.isConfigured)
        health.replace(api: client)
        identification = IdentificationStore(
            pipeline: Self.pipeline(
                client: client,
                configured: configuration.isConfigured,
                serviceID: configuration.baseURL?.absoluteString ?? "unconfigured"
            )
        )
    }

    /// The species half needs a service; the Vision half never does, so an
    /// unconfigured app still gates and still says what it can see.
    private static func pipeline(
        client: any PlantyAPI,
        configured: Bool,
        serviceID: String
    ) -> IdentificationPipeline {
        IdentificationPipeline(
            intake: PhotoLibraryIntake(),
            analyzer: VisionImageAnalyzer(),
            identifier: configured
                ? RemotePlantIdentifier(
                    api: client,
                    serviceID: serviceID
                )
                : UnavailableIdentifier(),
            cache: FileIdentificationCache()
        )
    }

    func storyStore(for plant: Plant) -> PlantStoryStore {
        let generation = apiGeneration
        return PlantStoryStore(
            api: api,
            plant: plant,
            isSessionCurrent: { [weak self] in self?.apiGeneration == generation }
        )
    }

    /// `asking` is sent the moment the screen opens, for entry points that
    /// already know the question and would only make the user retype it.
    func consultStore(for plant: Plant, asking question: String? = nil) -> ConsultStore {
        ConsultStore(api: api, plant: plant, pending: question)
    }

    /// Keeps the exact Today recommendation attached to the conversation while
    /// showing only the user's own words in the transcript.
    func consultStore(for entry: DigestEntry) -> ConsultStore {
        ConsultStore(
            api: api,
            plant: entry.plant,
            origin: .todayFinding(entry)
        )
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

    /// Notes about the place. Read before every answer about every plant,
    /// which is what makes them worth keeping apart from a plant's own.
    func householdNotesStore() -> NotesStore {
        NotesStore(api: api)
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

    func openPushRoute(_ route: PlantyPushRoute) {
        isShowingCareRound = false
        switch route {
        case .settings:
            break
        default:
            isShowingSettings = false
        }
        switch route {
        case .settings:
            isShowingSettings = true
        case .plant(let slug):
            selectedTab = .plants
            pendingPlantSlug = slug
        case .capture:
            selectedTab = .snap
        case .today:
            selectedTab = .today
        }
    }
}
