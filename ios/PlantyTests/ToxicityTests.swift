import Foundation
import Testing

@testable import Planty

/// The one rule this whole feature exists to honour: `unknown` must never
/// decode, rank or read as reassurance. Nobody checked is not the same as safe.
@Suite("Toxicity, and never calling unknown safe")
struct ToxicityTests {
    private func decode(_ json: String) throws -> Toxicity {
        try PlantyCoders.decoder().decode(Toxicity.self, from: Data(json.utf8))
    }

    @Test("The full record the service sends, snake_case keys and all")
    func fullRecord() throws {
        let toxicity = try decode(Self.lilyJSON)

        #expect(toxicity.cats == .severe)
        #expect(toxicity.dogs == .mild)
        #expect(toxicity.people == .mild)
        #expect(toxicity.basis == .source)
        #expect(toxicity.identifiedAs == "Lilium longiflorum")
        #expect(toxicity.principle == "unidentified nephrotoxin")
        #expect(toxicity.signs == "vomiting, then anuria")
        #expect(toxicity.parts == ["all", "flower"])
        #expect(toxicity.routes == ["eaten", "skin"])
        #expect(toxicity.notes == "why the audiences differ")
        #expect(toxicity.firstAid?.hasPrefix("a cat that groomed pollen") == true)
        #expect(toxicity.source == "www.aspca.org")
        #expect(toxicity.checkedAt != nil)
    }

    @Test("A missing rating decodes as unknown, never as absent and therefore fine")
    func missingRatingsAreUnknown() throws {
        let toxicity = try decode("{}")

        #expect(toxicity.cats == .unknown)
        #expect(toxicity.dogs == .unknown)
        #expect(toxicity.people == .unknown)
        #expect(toxicity.basis == .unknown)
        #expect(toxicity.isUnchecked)
    }

    /// A rating the app has never heard of is the same problem wearing a
    /// different hat: guessing at it is how a new grade reads as safe.
    @Test("A rating the app does not recognise falls back to unknown, not safe")
    func unrecognisedRatingIsUnknown() throws {
        let toxicity = try decode(#"{"cats": "probably fine", "dogs": "safe", "people": "safe"}"#)

        #expect(toxicity.cats == .unknown)
        #expect(toxicity.cats != .safe)
        #expect(toxicity.dogs == .safe)
    }

    @Test("Unknown never presents as safe or as nothing")
    func unknownPresentsAsUnchecked() {
        #expect(ToxicityRating.unknown.label == "Not checked")
        #expect(!ToxicityRating.unknown.isChecked)
        #expect(ToxicityRating.safe.isChecked)
        #expect(ToxicityRating.unknown.sentence(for: .cats).contains("Nobody has checked"))
    }

    /// The ordering is the safety property: a household where one audience was
    /// never checked can never headline on the strength of the other two.
    @Test("Unknown outranks safe, so a half-checked plant never headlines as safe")
    func unknownOutranksSafe() {
        #expect(ToxicityRating.unknown.severityOrder > ToxicityRating.safe.severityOrder)

        let halfChecked = Toxicity(cats: .unknown, dogs: .safe, people: .safe)
        #expect(halfChecked.worst == .unknown)
        #expect(halfChecked.headline == "Not checked")
        #expect(!halfChecked.isUnchecked, "one column is unknown, not the whole record")
    }

    @Test("Severe wins the headline outright")
    func severeWinsTheHeadline() {
        #expect(Toxicity.lily().worst == .severe)
        #expect(Toxicity.lily().headline == "Severely toxic")
    }

    /// The lily is the case that kills: a sore stomach for a dog, renal failure
    /// for a cat. Nobody should have to notice it by comparing three chips.
    @Test("Divergence is detected and said out loud")
    func divergenceIsSpelledOut() {
        let lily = Toxicity.lily()

        #expect(lily.diverges)
        #expect(lily.divergenceSentence == "Severe for cats, mild for dogs and people.")
    }

    @Test("An aroid agrees across the board and says nothing about divergence")
    func agreementIsQuiet() {
        let pothos = Toxicity.pothos()

        #expect(!pothos.diverges)
        #expect(pothos.divergenceSentence == nil)
        #expect(pothos.worst == .mild)
    }

    @Test("An unchecked column is named in the divergence sentence, never dropped")
    func divergenceNamesTheUncheckedColumn() {
        let partial = Toxicity(cats: .severe, dogs: .safe, people: .unknown)

        #expect(partial.divergenceSentence == "Severe for cats, not checked for people, safe for dogs.")
    }

    @Test("Nobody looked, and somebody looked and could not say, are different states")
    func lookedAtIsNotTheSameAsUnknown() {
        let never = Toxicity()
        #expect(never.isUnchecked)
        #expect(!never.isCheckedButUnresolved)

        let looked = Toxicity(checkedAt: .reference)
        #expect(!looked.isUnchecked)
        #expect(looked.isCheckedButUnresolved)
    }

    @Test("A derived grading is flagged as Planty's own call")
    func derivedIsFlagged() throws {
        let derived = try decode(#"{"cats": "moderate", "basis": "derived"}"#)

        #expect(derived.isDerived)
        #expect(!Toxicity.lily().isDerived, "the source graded that one itself")
    }

    @Test("A plant carries its toxicity through the detail envelope either way")
    func detailCarriesToxicity() throws {
        let onPlant = """
            {"plant": \(Self.plantWithToxicityJSON)}
            """
        let inEnvelope = """
            {"plant": \(ModelDecodingTests.monaJSON), "toxicity": \(Self.lilyJSON)}
            """

        let fromPlant = try PlantyCoders.decoder()
            .decode(PlantDetail.self, from: Data(onPlant.utf8))
        let fromEnvelope = try PlantyCoders.decoder()
            .decode(PlantDetail.self, from: Data(inEnvelope.utf8))

        #expect(fromPlant.plant.toxicity?.cats == .severe)
        #expect(fromEnvelope.toxicity?.cats == .severe)
    }

    @Test("A plant with no toxicity key at all decodes, and claims nothing")
    func absentToxicityIsNil() throws {
        let plant = try PlantyCoders.decoder()
            .decode(Plant.self, from: Data(ModelDecodingTests.monaJSON.utf8))

        #expect(plant.toxicity == nil)
    }

    @Test("A new edit cannot turn an unchecked record into an unsourced claim")
    func editRequiresBasis() {
        var form = ToxicityEditForm(plant: .fixture(), toxicity: nil)
        form.cats = .safe

        #expect(form.toxicity() == .failure(.missingBasis))
    }

    @Test("An urgent edit names the toxic principle")
    func urgentEditRequiresPrinciple() {
        var form = ToxicityEditForm(plant: .fixture(), toxicity: nil)
        form.cats = .severe
        form.basis = .source

        #expect(form.toxicity() == .failure(.missingPrinciple))
    }

    @Test("The edit form emits server enums in stable order and stamps the check")
    func editBuildsCompleteRecord() throws {
        let checkedAt = Date.reference
        var plant = Plant.fixture()
        plant.botanicalName = "Lilium longiflorum"
        var form = ToxicityEditForm(plant: plant, toxicity: nil)
        form.cats = .severe
        form.dogs = .mild
        form.people = .mild
        form.basis = .source
        form.principle = " unidentified nephrotoxin "
        form.parts = ["flower", "all"]
        form.routes = ["skin", "eaten"]

        let built = try form.toxicity(checkedAt: checkedAt).get()

        #expect(built.identifiedAs == plant.botanicalName)
        #expect(built.principle == "unidentified nephrotoxin")
        #expect(built.parts == ["all", "flower"])
        #expect(built.routes == ["eaten", "skin"])
        #expect(built.checkedAt == checkedAt)
    }
}

extension ToxicityTests {
    static let lilyJSON = """
        {
          "cats": "severe",
          "dogs": "mild",
          "people": "mild",
          "basis": "source",
          "identified_as": "Lilium longiflorum",
          "principle": "unidentified nephrotoxin",
          "signs": "vomiting, then anuria",
          "parts": ["all", "flower"],
          "routes": ["eaten", "skin"],
          "notes": "why the audiences differ",
          "first_aid": "a cat that groomed pollen goes to a vet before any sign appears",
          "source": "www.aspca.org",
          "checked_at": "2026-08-19T12:00:00Z"
        }
        """

    static let plantWithToxicityJSON = ModelDecodingTests.monaJSON.replacingOccurrences(
        of: "\"slug\": \"mona\"",
        with: "\"slug\": \"mona\", \"toxicity\": \(lilyJSON)"
    )
}
