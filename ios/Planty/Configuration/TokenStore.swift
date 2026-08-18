import Foundation
import Security

/// A bearer token is a credential, so it lives in the Keychain rather than in
/// UserDefaults where a file copy of the container would hand it over.
protocol TokenStoring: Sendable {
    func token(for account: String) -> String?
    func setToken(_ token: String?, for account: String)
}

struct KeychainTokenStore: TokenStoring {
    let service: String

    init(service: String = "zone.stout.Planty") {
        self.service = service
    }

    func token(for account: String) -> String? {
        var query = baseQuery(account: account)
        query[kSecReturnData as String] = true
        query[kSecMatchLimit as String] = kSecMatchLimitOne

        var item: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &item) == errSecSuccess,
              let data = item as? Data
        else { return nil }
        return String(data: data, encoding: .utf8)
    }

    func setToken(_ token: String?, for account: String) {
        let query = baseQuery(account: account)
        SecItemDelete(query as CFDictionary)

        guard let token, let data = token.data(using: .utf8) else { return }
        var insert = query
        insert[kSecValueData as String] = data
        insert[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
        SecItemAdd(insert as CFDictionary, nil)
    }

    private func baseQuery(account: String) -> [String: Any] {
        [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account
        ]
    }
}

/// In-memory stand-in so tests never touch the real Keychain.
final class InMemoryTokenStore: TokenStoring, @unchecked Sendable {
    private let lock = NSLock()
    private var storage: [String: String] = [:]

    init(seed: [String: String] = [:]) {
        storage = seed
    }

    func token(for account: String) -> String? {
        lock.withLock { storage[account] }
    }

    func setToken(_ token: String?, for account: String) {
        lock.withLock { storage[account] = token }
    }
}
