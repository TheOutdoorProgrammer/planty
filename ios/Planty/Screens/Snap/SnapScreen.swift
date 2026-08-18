import PhotosUI
import SwiftUI

/// The camera is the screen, not a button in a form. Plant selection sits on
/// top as a chip and never blocks the shutter.
struct SnapScreen: View {
    @Environment(AppSession.self) private var session
    @State private var camera = CameraController()
    @State private var isPickingPlant = false
    @State private var photoItem: PhotosPickerItem?
    @State private var isConfirmingDiscard = false
    @State private var route: [SnapRoute] = []

    private var store: CaptureStore { session.capture }

    var body: some View {
        NavigationStack(path: $route) {
            ScrollView {
                VStack(spacing: 18) {
                    PlantChip(plant: store.selectedPlant) { isPickingPlant = true }
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
                case .diagnosis(let plant, let photo):
                    DiagnosisScreen(store: session.diagnosisStore(plant: plant, photo: photo))
                }
            }
            .overlay(alignment: .top) { toast }
            .sheet(isPresented: $isPickingPlant) {
                PlantPickerSheet(plants: session.library.plants) { plant in
                    store.selectedPlant = plant
                    isPickingPlant = false
                }
            }
            .task(id: session.selectedTab) { await prepareIfVisible() }
            .onDisappear { camera.stop() }
            .onChange(of: photoItem) { _, item in
                Task { await load(item) }
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
        case .failed(_, let error):
            SaveFailedCard(error: error) {
                Task { await store.save(recording: nil) }
            } discard: {
                isConfirmingDiscard = true
            }
            capturedState
        }
    }

    @ViewBuilder
    private var readyState: some View {
        switch camera.availability {
        case .denied:
            CameraPermissionCard(photoItem: $photoItem)
        case .unavailable:
            NoCameraCard(photoItem: $photoItem)
        case .ready, .unknown:
            CameraStage(
                camera: camera,
                photoItem: $photoItem,
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
                identification: session.identification,
                useCandidate: { candidate in matchPlant(named: candidate) }
            )
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

    /// TabView builds neighbouring tabs eagerly, and asking for the camera
    /// before the user has opened Snap is how permission prompts get denied.
    private func prepareIfVisible() async {
        guard session.selectedTab == .snap else {
            camera.stop()
            return
        }
        await prepare()
    }

    private func prepare() async {
        if let context = session.snapContext {
            store.selectedPlant = context.plant
            session.snapContext = nil
        }
        if session.library.plants.isEmpty {
            await session.library.load()
        }
        await camera.prepare()
    }

    private func shoot() async {
        guard let jpeg = try? await camera.capture() else { return }
        store.accept(jpeg: jpeg)

        // A fresh capture has no asset, so it identifies but never caches.
        await session.identification.identify(jpeg: jpeg, assetID: nil)
    }

    private func load(_ item: PhotosPickerItem?) async {
        guard let item,
              let data = try? await item.loadTransferable(type: Data.self)
        else { return }
        store.accept(jpeg: data)
        photoItem = nil

        // itemIdentifier is the PHAsset localIdentifier, and the only thread
        // back to the original file's EXIF and GPS.
        await session.identification.identify(jpeg: data, assetID: item.itemIdentifier)
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
        route.append(.diagnosis(plant: plant, photo: photo))
    }
}

enum SnapRoute: Hashable {
    case diagnosis(plant: Plant, photo: CapturedPhoto)
}

extension CapturedPhoto: Hashable {
    static func == (lhs: CapturedPhoto, rhs: CapturedPhoto) -> Bool { lhs.id == rhs.id }
    func hash(into hasher: inout Hasher) { hasher.combine(id) }
}
