import PhotosUI
import SwiftUI

/// The camera is the screen, not a button in a form. Plant selection sits on
/// top as a chip and never blocks the shutter.
struct SnapScreen: View {
    @Environment(AppSession.self) private var session
    @State private var acquisition = PhotoAcquisition()
    @State private var isPickingPlant = false
    @State private var isConfirmingDiscard = false
    @State private var route: [SnapRoute] = []

    private var store: CaptureStore { session.capture }

    var body: some View {
        NavigationStack(path: $route) {
            ScrollView {
                VStack(spacing: 18) {
                    PlantChip(plant: store.selectedPlant) { isPickingPlant = true }
                    acquisitionError
                    stage
                }
                .padding(.horizontal, 20)
                .padding(.vertical, 16)
            }
            .plantyPage()
            .navigationTitle("Snap")
            .navigationBarTitleDisplayMode(.inline)
            .navigationDestination(for: SnapRoute.self) { route in
                switch route {
                case .aboutPlant(let plant, let photo):
                    ConsultScreen(store: session.photoConsultStore(plant: plant, photo: photo.jpeg))
                case .justAsk(let photo):
                    ConsultScreen(store: session.photoConsultStore(plant: nil, photo: photo.jpeg))
                }
            }
            .overlay(alignment: .top) { toast }
            .sheet(isPresented: $isPickingPlant) {
                PlantPickerSheet(plants: session.library.plants) { name in
                    isPickingPlant = false
                    Task {
                        // Identified, created and photographed in one step, so
                        // a first run never ends holding a picture of nothing.
                        let metadata = store.stage.photo
                            .map { CaptureMetadataReader.read(from: $0.jpeg) } ?? CaptureMetadata()
                        if await store.createPlant(named: name, metadata: metadata) != nil {
                            await session.library.load()
                        }
                    }
                } pick: { plant in
                    store.selectedPlant = plant
                    isPickingPlant = false
                }
            }
            .task(id: session.selectedTab) { await prepareIfVisible() }
            .onDisappear { acquisition.stop() }
            .onChange(of: acquisition.photoItem) { _, item in
                guard item != nil else { return }
                Task { await importPhoto() }
            }
            .confirmationDialog(
                "Discard this photo?",
                isPresented: $isConfirmingDiscard,
                titleVisibility: .visible
            ) {
                Button("Discard photo", role: .destructive) { store.discard() }
                Button("Keep it", role: .cancel) {}
            } message: {
                Text("The observation behind it is not saved anywhere else.")
            }
        }
    }

    @ViewBuilder
    private var stage: some View {
        switch store.stage {
        case .ready:
            readyState
        case .captured, .saving:
            capturedState
        case .failed(_, _, let error):
            // Retries whatever the user said they did, rather than saving the
            // photo alone and silently dropping the watering.
            SaveFailedCard(error: error) {
                Task {
                    if await store.retrySave() != nil {
                        await session.library.load()
                    }
                }
            } discard: {
                isConfirmingDiscard = true
            }
            capturedState
        }
    }

    @ViewBuilder
    private var readyState: some View {
        switch acquisition.camera.availability {
        case .denied:
            CameraPermissionCard(photoItem: photoItemBinding)
        case .unavailable:
            NoCameraCard(photoItem: photoItemBinding)
        case .ready, .unknown:
            CameraStage(
                camera: acquisition.camera,
                photoItem: photoItemBinding,
                shutter: { Task { await shoot() } }
            )
        }
    }

    @ViewBuilder
    private var capturedState: some View {
        if let photo = store.stage.photo {
            CapturedSheet(
                photo: photo,
                plant: store.selectedPlant,
                note: Binding(get: { store.note }, set: { store.note = $0 }),
                isBusy: store.stage.isBusy,
                record: { kind in Task { await store.save(recording: kind) } },
                lookOff: { startDiagnosis(photo: photo) },
                retake: { store.retake() },
                justAsk: { route.append(.justAsk(photo: photo)) },
                identification: session.identification,
                useCandidate: { candidate in matchPlant(named: candidate) }
            )
        }
    }

    @ViewBuilder
    private var acquisitionError: some View {
        if let error = acquisition.error {
            HStack(alignment: .top, spacing: 10) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .foregroundStyle(PlantyColor.orange)
                Text(error)
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.foreground)
                Spacer()
                Button("Dismiss") { acquisition.clearError() }
                    .font(.caption.weight(.semibold))
            }
            .padding(12)
            .background(PlantyColor.orange.opacity(0.12), in: RoundedRectangle(cornerRadius: 14))
        }
    }

    @ViewBuilder
    private var toast: some View {
        if let message = store.toast {
            SaveToast(message: message)
                .padding(.top, 6)
                .task {
                    try? await Task.sleep(for: .seconds(3))
                    store.clearToast()
                }
        }
    }

    private var photoItemBinding: Binding<PhotosPickerItem?> {
        Binding(
            get: { acquisition.photoItem },
            set: { acquisition.photoItem = $0 }
        )
    }

    /// TabView builds neighbouring tabs eagerly, and asking for the camera
    /// before the user has opened Snap is how permission prompts get denied.
    private func prepareIfVisible() async {
        guard session.selectedTab == .snap else {
            acquisition.stop()
            return
        }
        await prepare()
    }

    private func prepare() async {
        if let context = session.snapContext {
            store.selectedPlant = context.plant
            // Carried through the whole capture: settling it is what stops the
            // card coming back after the job was actually done.
            store.answering = context.verdictID
            session.snapContext = nil
        }
        if session.library.plants.isEmpty {
            await session.library.load()
        }
        await acquisition.prepare()
    }

    private func shoot() async {
        guard let acquired = await acquisition.takePhoto() else { return }
        await accept(acquired)
    }

    private func importPhoto() async {
        guard let acquired = await acquisition.importSelectedPhoto() else { return }
        await accept(acquired)
    }

    private func accept(_ acquired: AcquiredPhoto) async {
        store.accept(jpeg: acquired.jpeg)
        await session.identification.identify(jpeg: acquired.jpeg, assetID: acquired.assetID)
    }

    /// A candidate names a species, not one of your pots, so this only offers a
    /// match and never silently reassigns the photo.
    private func matchPlant(named candidate: IdentificationCandidate) {
        let wanted = candidate.commonName.lowercased()
        if let match = session.library.plants.first(where: {
            $0.commonName.lowercased() == wanted
                || $0.botanicalName?.lowercased() == candidate.scientificName?.lowercased()
        }) {
            store.selectedPlant = match
        } else {
            isPickingPlant = true
        }
    }

    /// Diagnosis is pushed, so the camera stays behind it in the stack.
    private func startDiagnosis(photo: CapturedPhoto) {
        guard let plant = store.selectedPlant else {
            isPickingPlant = true
            return
        }
        route.append(.aboutPlant(plant: plant, photo: photo))
    }
}

/// Both cases open the same chat. The difference is only whether a plant is
/// behind it, which decides whether the photo joins a story.
enum SnapRoute: Hashable {
    case aboutPlant(plant: Plant, photo: CapturedPhoto)

    /// Nothing is created, so the photo stays on the capture screen as it was.
    case justAsk(photo: CapturedPhoto)
}

extension CapturedPhoto: Hashable {
    static func == (lhs: CapturedPhoto, rhs: CapturedPhoto) -> Bool { lhs.id == rhs.id }
    func hash(into hasher: inout Hasher) { hasher.combine(id) }
}
