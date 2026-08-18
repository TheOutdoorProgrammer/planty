import Foundation

/// Where the service is and how to prove who we are. Never hardcoded: it comes
/// from the build (Info.plist) or from what the user typed in Settings.
struct PlantyConfiguration: Sendable, Equatable {
    var baseURL: URL?
    var token: String?

    /// Unconfigured is a real, first-class state. The app must say so plainly
    /// rather than showing a calm screen it has no evidence for.
    var isConfigured: Bool { baseURL != nil }

    static let unconfigured = PlantyConfiguration(baseURL: nil, token: nil)
}

/// Info.plist keys fed by the PLANTY_BASE_URL build setting, and friends.
enum ConfigurationKey {
    static let baseURLPlist = "PlantyBaseURL"
    static let tokenPlist = "PlantyAPIToken"
    static let baseURLDefaults = "planty.baseURL"
    static let tokenKeychainAccount = "planty.apiToken"
}

extension PlantyConfiguration {
    /// Resolution order: what the user set beats what the build baked in.
    static func resolve(
        bundle: Bundle = .main,
        defaults: UserDefaults = .standard,
        tokens: TokenStoring = KeychainTokenStore()
    ) -> PlantyConfiguration {
        let overrideURL = trimmed(defaults.string(forKey: ConfigurationKey.baseURLDefaults))
        let bakedURL = trimmed(bundle.object(forInfoDictionaryKey: ConfigurationKey.baseURLPlist) as? String)
        let overrideToken = trimmed(tokens.token(for: ConfigurationKey.tokenKeychainAccount))
        let bakedToken = trimmed(bundle.object(forInfoDictionaryKey: ConfigurationKey.tokenPlist) as? String)

        return PlantyConfiguration(
            baseURL: (overrideURL ?? bakedURL).flatMap(URL.init(string:)),
            token: overrideToken ?? bakedToken
        )
    }

    /// An unsubstituted build setting arrives as the literal empty string.
    private static func trimmed(_ value: String?) -> String? {
        guard let value else { return nil }
        let clean = value.trimmingCharacters(in: .whitespacesAndNewlines)
        return clean.isEmpty ? nil : clean
    }
}
