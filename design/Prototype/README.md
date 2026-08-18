# Planty SwiftUI prototype

This is a local, view-only iOS 26 prototype for the four core Planty experiences: Today, Snap, Diagnosis, and a plant story.
It contains realistic mock data but no camera integration, networking, or persistence.

Open `PlantyPrototype.xcodeproj` in Xcode 26.5 or newer and run the `PlantyPrototype` scheme on an iOS 26 simulator.

The Today screen opens in its normal calm state.
Use the half-filled circle button in the navigation bar to switch to the needs-care state.
The Snap tab uses a mock camera surface; tap the shutter to reveal the quick record actions, then choose “Something looks off” to open the diagnosis conversation.
The Plants tab opens Mona's story directly because the plant-library shell is outside this prototype's four-screen scope.

To verify from the command line:

```zsh
xcodebuild \
  -project PlantyPrototype.xcodeproj \
  -scheme PlantyPrototype \
  -configuration Debug \
  -sdk iphonesimulator \
  -destination 'generic/platform=iOS Simulator' \
  -derivedDataPath .build \
  CODE_SIGNING_ALLOWED=NO \
  build
```

The app uses the existing `../logo/planty-mark.svg` as a bundled resource.
All other visuals are native SwiftUI and SF Symbols, so there are no third-party dependencies.
