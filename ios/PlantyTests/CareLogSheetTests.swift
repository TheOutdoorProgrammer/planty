import Testing

@testable import Planty

@Suite("Care log constraints")
struct CareLogSheetTests {
    @Test("A follow-up cannot submit a different care kind")
    @MainActor
    func fixedKindWins() {
        let submitted = CareLogSheet.submittedKind(
            selected: .misted,
            fixed: .fertilized
        )

        #expect(submitted == .fertilized)
    }

    @Test("The general care logger keeps the selected kind")
    @MainActor
    func generalLoggerKeepsSelection() {
        let submitted = CareLogSheet.submittedKind(
            selected: .misted,
            fixed: nil
        )

        #expect(submitted == .misted)
    }
}
