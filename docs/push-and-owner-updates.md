# Push notifications and owner updates

## Push notifications

Planty now sends the operator's notifications directly through Apple Push Notification service (APNs). Home Assistant remains responsible for sensors, weather and actuators.

The iOS app asks for notification permission, registers its APNs device token with `POST /v1/push-devices`, and refreshes that registration whenever the Planty service URL changes.

The server needs an Apple Push Notification auth key from Certificates, Identifiers & Profiles:

- `PLANTY_APNS_KEY_ID`
- `PLANTY_APNS_TEAM_ID`
- `PLANTY_APNS_PRIVATE_KEY`
- `PLANTY_APNS_BUNDLE_ID` (defaults to `zone.stout.Planty`)
- `PLANTY_APNS_ENVIRONMENT` (`production` by default, or `sandbox`)

These are not the App Store Connect API credentials used by the release workflow.

When APNs is configured, notifications addressed to Planty's primary notifier service go to APNs first and Home Assistant announcements are suppressed. A named backup-person notifier still goes through Home Assistant. If APNs cannot deliver at all, the existing HA notify service is used as a failure fallback so a configuration rollout cannot silently lose an alert.

The release workflow signs Planty with the production `aps-environment` entitlement. The `zone.stout.Planty` App ID must have Push Notifications enabled in the Apple Developer portal so automatic provisioning can include that entitlement.

## Update person

The More/Garden screen shows an **Update <person>** action for each steward who owns plants currently in Planty.

The service gathers exactly the previous seven days of observations and daily verdicts for that person's active plants, adds the latest photo metadata and visual findings, and asks the configured Claude backend for an 80–180 word owner-facing summary. The response also contains a short-lived URL for the newest photo of every plant that has one.

The iOS app downloads those photos, lets the caretaker edit the generated text, remembers the owner's phone number locally on that iPhone, and opens the native Messages composer with the text and photos already attached. iOS deliberately requires the user to tap **Send**; apps cannot silently send an iMessage/SMS on a person's behalf.
