import MessageUI
import SwiftUI
import UniformTypeIdentifiers

struct OwnerUpdateTarget: Identifiable {
    let name: String
    var id: String { name }
}

struct OwnerUpdateFlow: View {
    let steward: String
    let store: GardenStore

    @Environment(\.dismiss) private var dismiss
    @State private var summary = ""
    @State private var phone: String
    @State private var attachments: [MessageAttachment] = []
    @State private var photoCount = 0
    @State private var isLoading = true
    @State private var error: PlantyError?
    @State private var isShowingMessages = false

    init(steward: String, store: GardenStore) {
        self.steward = steward
        self.store = store
        _phone = State(initialValue: UserDefaults.standard.string(forKey: Self.phoneKey(steward)) ?? "")
    }

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 18) {
                    if isLoading {
                        loading
                    } else if let error {
                        StateMessage(
                            title: "Could not prepare the update",
                            message: error.errorDescription ?? "Try again in a moment.",
                            accent: PlantyColor.orange,
                            icon: "exclamationmark.triangle.fill"
                        ) {
                            Button("Try again") { Task { await prepare() } }
                                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.orange))
                        }
                    } else {
                        prepared
                    }
                }
                .padding(16)
            }
            .plantyPage()
            .navigationTitle("Update \(steward)")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Close") { dismiss() }
                }
            }
            .task { await prepare() }
            .sheet(isPresented: $isShowingMessages) {
                MessageComposer(
                    recipients: phone.cleaned.isEmpty ? [] : [phone.cleaned],
                    body: summary.cleaned,
                    attachments: attachments
                )
            }
        }
    }

    private var loading: some View {
        VStack(spacing: 14) {
            ProgressView().tint(PlantyColor.purple)
            Text("Reading the last seven days…")
                .font(.headline)
            Text("Planty is gathering care records and each plant's latest photo, then Claude will turn that into a short update.")
                .font(.subheadline)
                .foregroundStyle(PlantyColor.secondaryText)
                .multilineTextAlignment(.center)
        }
        .frame(maxWidth: .infinity)
        .plantyCard(border: PlantyColor.purple.opacity(0.18))
    }

    private var prepared: some View {
        VStack(alignment: .leading, spacing: 18) {
            VStack(alignment: .leading, spacing: 8) {
                SectionHeading("Message", detail: "Generated from the previous seven days. Edit anything before sending.")
                TextEditor(text: $summary)
                    .frame(minHeight: 180)
                    .padding(10)
                    .background(PlantyColor.elevated, in: RoundedRectangle(cornerRadius: 14))
                    .overlay {
                        RoundedRectangle(cornerRadius: 14)
                            .stroke(PlantyColor.quietDecoration.opacity(0.25), lineWidth: 1)
                    }
            }

            VStack(alignment: .leading, spacing: 8) {
                SectionHeading("Recipient", detail: "Saved only on this iPhone for the next update.")
                TextField("Phone number", text: $phone)
                    .textContentType(.telephoneNumber)
                    .keyboardType(.phonePad)
                    .padding(12)
                    .background(PlantyColor.elevated, in: RoundedRectangle(cornerRadius: 14))
            }

            Label(
                attachmentLine,
                systemImage: attachments.isEmpty ? "photo" : "photo.stack.fill"
            )
            .font(.subheadline)
            .foregroundStyle(PlantyColor.secondaryText)

            if MFMessageComposeViewController.canSendText() {
                Button("Open in Messages") {
                    let cleaned = phone.cleaned
                    if !cleaned.isEmpty {
                        UserDefaults.standard.set(cleaned, forKey: Self.phoneKey(steward))
                    }
                    isShowingMessages = true
                }
                .buttonStyle(PrimaryButtonStyle(color: PlantyColor.green))
                .disabled(summary.cleaned.isEmpty)
            } else {
                StateMessage(
                    title: "Messages is not available here",
                    message: "The composer works on an iPhone with Messages configured. The generated update is still above.",
                    accent: PlantyColor.orange,
                    icon: "message.badge.filled.fill"
                ) { EmptyView() }
            }
        }
    }

    private var attachmentLine: String {
        switch (attachments.count, photoCount) {
        case (0, 0): return "No plant has a photo yet."
        case let (attached, total) where attached == total:
            return "\(attached) latest photo\(attached == 1 ? "" : "s") will be attached."
        case let (attached, total):
            return "\(attached) of \(total) latest photos could be attached."
        }
    }

    private func prepare() async {
        isLoading = true
        error = nil
        do {
            let update = try await store.ownerUpdate(steward: steward)
            summary = update.summary
            photoCount = update.photos.count
            attachments = await download(update.photos)
            isLoading = false
        } catch {
            self.error = PlantyError.from(error)
            isLoading = false
        }
    }

    private func download(_ photos: [OwnerUpdatePhoto]) async -> [MessageAttachment] {
        var result: [MessageAttachment] = []
        for photo in photos {
            do {
                guard let url = photo.url.validatedRemoteImageURL else { continue }
                let (data, response) = try await URLSession.shared.data(from: url)
                guard let http = response as? HTTPURLResponse,
                      (200..<300).contains(http.statusCode), !data.isEmpty else { continue }
                result.append(MessageAttachment(
                    data: data,
                    typeIdentifier: UTType.jpeg.identifier,
                    filename: "\(photo.plantSlug)-latest.jpg"
                ))
            } catch {
                continue
            }
        }
        return result
    }

    private static func phoneKey(_ steward: String) -> String {
        "planty.owner-phone.\(steward)"
    }
}

struct MessageAttachment: Identifiable {
    let id = UUID()
    let data: Data
    let typeIdentifier: String
    let filename: String
}

struct MessageComposer: UIViewControllerRepresentable {
    let recipients: [String]
    let body: String
    let attachments: [MessageAttachment]

    func makeCoordinator() -> Coordinator { Coordinator() }

    func makeUIViewController(context: Context) -> MFMessageComposeViewController {
        let controller = MFMessageComposeViewController()
        controller.messageComposeDelegate = context.coordinator
        controller.recipients = recipients
        controller.body = body
        for attachment in attachments {
            controller.addAttachmentData(
                attachment.data,
                typeIdentifier: attachment.typeIdentifier,
                filename: attachment.filename
            )
        }
        return controller
    }

    func updateUIViewController(_ uiViewController: MFMessageComposeViewController, context: Context) {}

    final class Coordinator: NSObject, MFMessageComposeViewControllerDelegate {
        func messageComposeViewController(
            _ controller: MFMessageComposeViewController,
            didFinishWith result: MessageComposeResult
        ) {
            controller.dismiss(animated: true)
        }
    }
}
