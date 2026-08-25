import Foundation

/// Intake, metadata, cutout, gate, species, cache. Every step degrades on its
/// own: no permission still identifies, offline still gates, and a dead species
/// backend still shows coarse labels.
struct IdentificationPipeline: Sendable {
    let intake: any PhotoIntaking
    let analyzer: any ImageAnalyzing
    let identifier: any PlantIdentifying
    let cache: any IdentificationCaching

    /// Injected so tests do not depend on the clock.
    var now: @Sendable () -> Date = { Date() }

    func identify(pickedData: Data, assetID: String?) async -> IdentificationOutcome {
        if let assetID,
           let cached = await cache.result(for: assetID),
           cached.backend == identifier.backendID {
            return .identified(cached)
        }

        let photo = await intake.intake(pickedData: pickedData, assetID: assetID)

        // The cutout is an optimisation, not a requirement: if Vision finds no
        // foreground the raw frame is still worth classifying.
        let subject = (try? await analyzer.isolateSubject(in: photo.data)) ?? nil
        let toClassify = subject ?? photo.data

        let labels = (try? await analyzer.classify(toClassify)) ?? []
        let presentable = PlantGate.presentable(labels)

        // The gate is a cost control before it is a nicety: it stops a paid
        // species call on a photo of a dog or a receipt.
        guard labels.isEmpty || PlantGate.verdict(for: labels) == .plant else {
            return .notAPlant(coarse: presentable)
        }

        do {
            let candidates = try await identifier.identify(
                imageData: toClassify,
                metadata: photo.metadata
            )
            guard !candidates.isEmpty else {
                return .coarseOnly(coarse: presentable, reason: "Nothing came back confident enough to name.")
            }

            let result = PlantIdentification(
                assetID: photo.assetID ?? "",
                candidates: candidates.sorted { $0.confidence > $1.confidence },
                coarse: presentable,
                metadata: photo.metadata,
                backend: identifier.backendID,
                identifiedAt: now()
            )
            if photo.assetID != nil {
                await cache.store(result)
            }
            return .identified(result)
        } catch {
            return .coarseOnly(
                coarse: presentable,
                reason: PlantyError.from(error).errorDescription
                    ?? "The species service could not be reached."
            )
        }
    }
}
