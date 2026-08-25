import Foundation
import Testing

@testable import Planty

@MainActor
@Suite("Printable plant labels")
struct PlantLabelTests {
    @Test("A label URL round-trips punctuation in a slug")
    func deepLinkRoundTrip() throws {
        let plant = Plant.fixture(slug: "joeys-pothos", commonName: "Joey's pothos")
        let url = PlantDeepLink.url(for: plant)

        #expect(url.absoluteString == "planty://plant/joeys-pothos")
        #expect(PlantDeepLink.plantSlug(from: url) == plant.slug)
        #expect(PlantDeepLink.plantSlug(from: URL(string: "https://example.com/plant/x")!) == nil)
    }

    @Test("Opening a label reveals the plant instead of staying behind Settings")
    func routesToPlant() {
        let session = AppSession(
            defaults: UserDefaults(suiteName: UUID().uuidString)!,
            tokens: InMemoryTokenStore(),
            api: FakeAPI()
        )
        session.isShowingSettings = true

        session.openDeepLink(URL(string: "planty://plant/mona")!)

        #expect(!session.isShowingSettings)
        #expect(session.selectedTab == .plants)
        #expect(session.pendingPlantSlug == "mona")
    }

    @Test("The printable artifact is a nonempty PDF")
    func rendersPDF() throws {
        let plant = Plant.fixture(slug: "mona", commonName: "Mona")
        let url = try PlantLabelRenderer.pdf(for: plant, deepLink: PlantDeepLink.url(for: plant))
        let data = try Data(contentsOf: url)

        #expect(data.starts(with: Data("%PDF".utf8)))
        #expect(data.count > 1_000)
    }
}
