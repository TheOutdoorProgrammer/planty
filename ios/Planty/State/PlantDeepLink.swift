import Foundation

enum PlantDeepLink {
    static let careRoundURL = URL(string: "planty://care-round")!

    static func url(for plant: Plant) -> URL {
        var components = URLComponents()
        components.scheme = "planty"
        components.host = "plant"
        components.path = "/\(plant.slug)"
        return components.url!
    }

    static func plantSlug(from url: URL) -> String? {
        guard url.scheme?.lowercased() == "planty",
              url.host?.lowercased() == "plant"
        else { return nil }
        let slug = url.pathComponents.dropFirst().joined(separator: "/")
            .removingPercentEncoding?
            .trimmingCharacters(in: .whitespacesAndNewlines)
        return slug?.isEmpty == false ? slug : nil
    }

    static func opensCareRound(_ url: URL) -> Bool {
        url.scheme?.lowercased() == "planty" && url.host?.lowercased() == "care-round"
    }
}
extension AppSession {
    func openDeepLink(_ url: URL) {
        if PlantDeepLink.opensCareRound(url) {
            isShowingSettings = false
            selectedTab = .today
            isShowingCareRound = true
            return
        }
        guard let slug = PlantDeepLink.plantSlug(from: url) else { return }
        isShowingSettings = false
        selectedTab = .plants
        pendingPlantSlug = slug
    }
}
