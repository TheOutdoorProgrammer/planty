import Foundation
import Testing

@testable import Planty

@Suite("Photo URL validation")
struct PhotoURLTests {
    @Test("Only absolute HTTP image links are renderable")
    func validatesRemoteURLs() {
        #expect(URL(string: "https://planty.test/photo.jpg")?.validatedRemoteImageURL != nil)
        #expect(URL(string: "http://planty.test/photo.jpg")?.validatedRemoteImageURL != nil)
        #expect(URL(string: "ftp://planty.test/photo.jpg")?.validatedRemoteImageURL == nil)
        #expect(URL(string: "/photo.jpg")?.validatedRemoteImageURL == nil)
    }
}
