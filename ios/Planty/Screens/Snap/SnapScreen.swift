import PhotosUI
import SwiftUI

/// Capture is a focused task. The selected plant is always visible, the camera
/// owns the screen while shooting, and the follow-up choices become scrollable
/// only after a photo exists.
struct SnapScreen: View {
    @Environment(AppSession.self) private var session
    @State private var acquisition = PhotoAcquisition()
    @State private var isPickingPlant = false
    @State private var isConfirmingDiscard = false
    @State private var route: [SnapRoute] = []

    private var store: CaptureStore { session.capture }

    var body: some View {
        NavigationStack(path: $route) {
            Group {
                if store.stage.photo == nil {
                    readyLayout
                } else {
                    capturedLayout
                }
            }
            .plantyPage()
            .navigationTitle("Capture")
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

    private var readyLayout: some View {
        ScrollView {
            VStack(spacing: 14) {
                PlantChip(plant: store.selectedPlant) { isPickingPlant = true }
                acquisitionError
                readyState
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
        }
    }

    private var capturedLayout: some View {
        ScrollView {
            VStack(spacing: 14) {
                PlantChip(plant: store.selectedPlant) { isPickingPlant = true }
                acquisitionError
                stage
            }
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
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
            .plantyCard(border: PlantyColor.orange.opacity(0.2), padding: 12)
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

    private func startDiagnosis(photo: CapturedPhoto) {
        guard let plant = store.selectedPlant else {
            isPickingPlant = true
            return
        }
        route.append(.aboutPlant(plant: plant, photo: photo))
    }
}

enum SnapRoute: Hashable {
    case aboutPlant(plant: Plant, photo: CapturedPhoto)
    case justAsk(photo: CapturedPhoto)
}

extension CapturedPhoto: Hashable {
    static func == (lhs: CapturedPhoto, rhs: CapturedPhoto) -> Bool { lhs.id == rhs.id }
    func hash(into hasher: inout Hasher) { hasher.combine(id) }
}
