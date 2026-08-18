import SwiftUI

/// "Not now" postpones for an explicit interval. It never quietly marks the
/// task complete, which is a different claim entirely.
struct PostponeSheet: View {
    let entry: DigestEntry
    let postpone: (PostponeInterval) -> Void
    let handled: () -> Void

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 14) {
                Text("\(entry.plant.commonName) can wait a bit.")
                    .font(.title3.weight(.bold))
                Text("Planty will bring this back. Nothing is marked done.")
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)

                ForEach(PostponeInterval.allCases) { interval in
                    Button(interval.label) { postpone(interval) }
                        .buttonStyle(SecondaryButtonStyle())
                }

                Button("I already handled it", action: handled)
                    .buttonStyle(PrimaryButtonStyle(color: PlantyColor.green))

                Spacer(minLength: 0)
            }
            .padding(20)
            .frame(maxWidth: .infinity, alignment: .leading)
            .plantyPage()
            .navigationTitle("Not now")
            .navigationBarTitleDisplayMode(.inline)
        }
        .presentationDetents([.medium])
    }
}

/// One tap, no typing. "I already handled it" still has to say what happened,
/// because the record is what future diagnoses read.
struct HandledSheet: View {
    let entry: DigestEntry
    let record: (ObservationKind) -> Void

    private var options: [ObservationKind] {
        switch entry.verdict.action {
        case .water: [.watered, .note]
        case .harvest: [.harvested, .note]
        case .check, .urgent: [.symptom, .moved, .note]
        case .none, .unknown: [.note]
        }
    }

    var body: some View {
        NavigationStack {
            VStack(alignment: .leading, spacing: 14) {
                Text("What did you do?")
                    .font(.title3.weight(.bold))
                Text("One tap is enough. No note required.")
                    .font(.subheadline)
                    .foregroundStyle(PlantyColor.secondaryText)

                ForEach(options, id: \.self) { kind in
                    Button {
                        record(kind)
                    } label: {
                        Label(kind.label, systemImage: kind.symbol)
                    }
                    .buttonStyle(SecondaryButtonStyle())
                }

                Spacer(minLength: 0)
            }
            .padding(20)
            .frame(maxWidth: .infinity, alignment: .leading)
            .plantyPage()
            .navigationTitle(entry.plant.commonName)
            .navigationBarTitleDisplayMode(.inline)
        }
        .presentationDetents([.medium])
    }
}
