import Foundation
import Testing

@testable import Planty

@Suite("Where the base URL and token come from")
struct ConfigurationTests {
    private func defaults(_ name: String = UUID().uuidString) -> UserDefaults {
        UserDefaults(suiteName: name) ?? .standard
    }

    @Test("Nothing anywhere is an explicit unconfigured state")
    func nothingConfigured() {
        let config = PlantyConfiguration.resolve(
            bundle: Bundle(for: StubURLProtocol.self),
            defaults: defaults(),
            tokens: InMemoryTokenStore()
        )
        #expect(!config.isConfigured)
        #expect(config.baseURL == nil)
    }

    @Test("What the user typed beats what the build baked in")
    func userOverridesBuild() {
        let store = defaults()
        store.set("https://typed.example", forKey: ConfigurationKey.baseURLDefaults)
        let tokens = InMemoryTokenStore(seed: [ConfigurationKey.tokenKeychainAccount: "typed"])

        let config = PlantyConfiguration.resolve(
            bundle: Bundle(for: StubURLProtocol.self),
            defaults: store,
            tokens: tokens
        )
        #expect(config.baseURL?.absoluteString == "https://typed.example")
        #expect(config.token == "typed")
        #expect(config.isConfigured)
    }

    @Test("An unsubstituted build setting arrives empty and must not configure")
    func emptyBuildSettingIsNotConfiguration() {
        let store = defaults()
        store.set("   ", forKey: ConfigurationKey.baseURLDefaults)

        let config = PlantyConfiguration.resolve(
            bundle: Bundle(for: StubURLProtocol.self),
            defaults: store,
            tokens: InMemoryTokenStore()
        )
        #expect(!config.isConfigured)
    }

    @Test("Clearing the token removes it rather than storing an empty string")
    func clearingToken() {
        let tokens = InMemoryTokenStore(seed: [ConfigurationKey.tokenKeychainAccount: "old"])
        tokens.setToken(nil, for: ConfigurationKey.tokenKeychainAccount)
        #expect(tokens.token(for: ConfigurationKey.tokenKeychainAccount) == nil)
    }

    @Test("The token is never written to UserDefaults")
    func tokenStaysOutOfDefaults() {
        let store = defaults()
        let tokens = InMemoryTokenStore()
        tokens.setToken("s3cret", for: ConfigurationKey.tokenKeychainAccount)

        _ = PlantyConfiguration.resolve(
            bundle: Bundle(for: StubURLProtocol.self),
            defaults: store,
            tokens: tokens
        )
        let leaked = store.dictionaryRepresentation().values.contains { value in
            (value as? String) == "s3cret"
        }
        #expect(!leaked)
    }
}

@Suite("Completion copy")
struct CompletionCopyTests {
    @Test("Clearing the last card says so, rather than pretending it never was")
    func justFinished() {
        let digest = Digest.fixture(date: Date.reference.minus(hours: 1))
        let presentation = TodayPresentation.make(
            .fixture(digest: digest, didJustFinish: true)
        )
        guard case .calm(let summary) = presentation else {
            Issue.record("expected calm")
            return
        }
        #expect(summary.headline == "That's it.")
        #expect(summary.subhead == "Everything else is okay.")
    }

    @Test("A plain calm day uses the default copy")
    func plainCalm() {
        let digest = Digest.fixture(date: Date.reference.minus(hours: 1))
        guard case .calm(let summary) = TodayPresentation.make(.fixture(digest: digest)) else {
            Issue.record("expected calm")
            return
        }
        #expect(summary.headline == "You're done.")
        #expect(summary.subhead == "Nothing needs you right now.")
    }
}
