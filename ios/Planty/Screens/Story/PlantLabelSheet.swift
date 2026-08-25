import CoreImage
import CoreImage.CIFilterBuiltins
import SwiftUI
import UIKit

struct PlantLabelSheet: View {
    let plant: Plant

    @Environment(\.dismiss) private var dismiss
    @State private var documentURL: URL?
    @State private var failure: String?

    private var deepLink: URL { PlantDeepLink.url(for: plant) }

    var body: some View {
        NavigationStack {
            VStack(spacing: 20) {
                labelPreview

                Text("Scanning this label opens \(plant.commonName) directly in Planty.")
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)
                    .multilineTextAlignment(.center)

                if let documentURL {
                    ShareLink(
                        item: documentURL,
                        preview: SharePreview("\(plant.commonName) Planty label")
                    ) {
                        Label("Print or share PDF", systemImage: "printer.fill")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(PrimaryButtonStyle(color: PlantyColor.green))
                } else if let failure {
                    Text(failure)
                        .font(.footnote)
                        .foregroundStyle(PlantyColor.orange)
                } else {
                    ProgressView("Making label…")
                }
            }
            .padding(24)
            .plantyReadableContent(maxWidth: 520)
            .plantyPage()
            .navigationTitle("QR label")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("Done") { dismiss() }
                }
            }
            .task { makeDocument() }
        }
    }

    private var labelPreview: some View {
        VStack(spacing: 12) {
            Text(plant.commonName)
                .font(.title2.weight(.bold))
                .multilineTextAlignment(.center)
            if let species = plant.displaySpecies, !species.isEmpty {
                Text(species)
                    .font(.subheadline.italic())
                    .foregroundStyle(PlantyColor.secondaryText)
            }
            if let qr = PlantLabelRenderer.qrCode(for: deepLink, side: 220) {
                Image(uiImage: qr)
                    .interpolation(.none)
                    .resizable()
                    .scaledToFit()
                    .frame(width: 220, height: 220)
                    .accessibilityLabel("QR code for \(plant.commonName)")
            }
        }
        .frame(maxWidth: .infinity)
        .padding(24)
        .background(Color.white, in: RoundedRectangle(cornerRadius: 18, style: .continuous))
        .foregroundStyle(Color.black)
    }

    private func makeDocument() {
        do {
            documentURL = try PlantLabelRenderer.pdf(for: plant, deepLink: deepLink)
        } catch {
            failure = "Planty could not make the PDF. \(error.localizedDescription)"
        }
    }
}
enum PlantLabelRenderer {
    private static let context = CIContext(options: [.useSoftwareRenderer: false])

    static func qrCode(for url: URL, side: CGFloat) -> UIImage? {
        let filter = CIFilter.qrCodeGenerator()
        filter.message = Data(url.absoluteString.utf8)
        filter.correctionLevel = "M"
        guard let output = filter.outputImage else { return nil }
        let extent = output.extent.integral
        let scale = max(1, floor(side / max(extent.width, extent.height)))
        let transformed = output.transformed(by: CGAffineTransform(scaleX: scale, y: scale))
        guard let image = context.createCGImage(transformed, from: transformed.extent) else { return nil }
        return UIImage(cgImage: image)
    }

    static func pdf(for plant: Plant, deepLink: URL) throws -> URL {
        let page = CGRect(x: 0, y: 0, width: 288, height: 180)
        let renderer = UIGraphicsPDFRenderer(bounds: page)
        let data = renderer.pdfData { context in
            context.beginPage()
            UIColor.white.setFill()
            context.cgContext.fill(page)

            let title = NSAttributedString(
                string: plant.commonName,
                attributes: [
                    .font: UIFont.systemFont(ofSize: 18, weight: .bold),
                    .foregroundColor: UIColor.black
                ]
            )
            title.draw(in: CGRect(x: 18, y: 20, width: 150, height: 48))

            let subtitle = NSAttributedString(
                string: [plant.displaySpecies, plant.location]
                    .compactMap { $0 }
                    .filter { !$0.isEmpty }
                    .joined(separator: " · "),
                attributes: [
                    .font: UIFont.systemFont(ofSize: 10),
                    .foregroundColor: UIColor.darkGray
                ]
            )
            subtitle.draw(in: CGRect(x: 18, y: 70, width: 150, height: 42))

            if let qr = qrCode(for: deepLink, side: 132) {
                qr.draw(in: CGRect(x: 144, y: 24, width: 120, height: 120))
            }
            let footer = NSAttributedString(
                string: "Scan to open in Planty",
                attributes: [
                    .font: UIFont.systemFont(ofSize: 9, weight: .semibold),
                    .foregroundColor: UIColor.black
                ]
            )
            footer.draw(in: CGRect(x: 18, y: 146, width: 246, height: 18))
        }

        let safeName = plant.slug.replacingOccurrences(of: "/", with: "-")
        let url = FileManager.default.temporaryDirectory
            .appendingPathComponent("planty-label-\(safeName).pdf")
        try data.write(to: url, options: .atomic)
        return url
    }
}
