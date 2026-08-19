import SwiftUI

/// The record, read only and folded away. Everything here is editable from the
/// menu; this is for checking what Planty thinks it knows without opening a
/// form and risking changing it.
struct PlantFactsCard: View {
    let plant: Plant
    @State private var open = false

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Button {
                withAnimation(.snappy) { open.toggle() }
            } label: {
                HStack(spacing: 6) {
                    Image(systemName: open ? "chevron.down" : "chevron.right")
                        .font(.caption2.weight(.semibold))
                    Text("What Planty knows about it")
                        .font(.subheadline.weight(.semibold))
                    Spacer()
                    if !open {
                        Text(summary)
                            .font(.caption)
                            .foregroundStyle(PlantyColor.secondaryText)
                            .lineLimit(1)
                    }
                }
                .foregroundStyle(PlantyColor.foreground)
                .frame(minHeight: 44)
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)

            if open {
                VStack(alignment: .leading, spacing: 8) {
                    ForEach(facts, id: \.label) { fact in
                        LabeledContent {
                            Text(fact.value)
                                .font(.subheadline)
                                .foregroundStyle(PlantyColor.foreground)
                                .multilineTextAlignment(.trailing)
                        } label: {
                            Text(fact.label)
                                .font(.caption)
                                .foregroundStyle(PlantyColor.secondaryText)
                        }
                    }
                }
                .transition(.opacity.combined(with: .move(edge: .top)))
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .plantyCard(padding: 14)
    }

    /// One line when folded, so the card is worth having closed.
    private var summary: String {
        var parts = [plant.location.isEmpty ? "no place recorded" : plant.location]
        if plant.isSheltered { parts.append("indoors") }
        return parts.joined(separator: " · ")
    }

    private var facts: [(label: String, value: String)] {
        var out: [(String, String)] = []

        out.append(("Where", plant.location.isEmpty ? "Not recorded" : plant.location))
        if let area = plant.haArea, !area.isEmpty {
            out.append(("Home Assistant area", area))
        }
        out.append(("Right now", plant.isSheltered ? "Indoors" : "Where it lives"))
        out.append(("Kind", plant.domain.label))
        out.append(("Looked after by", plant.steward == "self" ? "You" : plant.steward))
        out.append(("How it is doing", plant.status.editLabel))
        out.append(("How hard to reach", plant.accessibility.editLabel))
        out.append(("Watering", plant.wateringMethod.label))

        if let light = plant.lightExposure, light != .unknown {
            out.append(("Light it gets", light.label))
        }
        out.append(("Cold limit", coldLimit))

        if let size = plant.potSizeIn {
            out.append(("Pot", potLine(size)))
        } else if let material = plant.potMaterial, !material.isEmpty {
            out.append(("Pot", material))
        }
        if let drains = plant.hasDrainage {
            out.append(("Drainage hole", drains ? "Yes" : "No"))
        }
        if let acquired = plant.acquiredAt {
            out.append(("Since", acquired.formatted(date: .abbreviated, time: .omitted)))
        }
        return out
    }

    /// Named rather than left blank: without it the cold watch never considers
    /// this plant at all, and silence there reads as safety.
    private var coldLimit: String {
        guard let min = plant.minTempF else {
            return "Not set, so Planty never warns about frost"
        }
        return "\(Int(min))F"
    }

    private func potLine(_ size: Double) -> String {
        let inches = size.rounded() == size ? String(Int(size)) : String(format: "%.1f", size)
        guard let material = plant.potMaterial, !material.isEmpty else { return "\(inches) inch" }
        return "\(inches) inch \(material)"
    }
}
