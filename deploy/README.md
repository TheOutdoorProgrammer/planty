# Deploying Planty

These manifests are **not** applied.
Flux reconciles whatever lands in the `flux` repo, so committing them there is the same act as deploying.
They live here until you have read them.

## Moving them into Flux

```sh
FLUX=<your-flux-repo>/clusters/<cluster>/planty
mkdir -p "$FLUX"
cp deploy/*.yaml "$FLUX"/
rm "$FLUX/secret.yaml.example"      # the example never belongs in the cluster
```

Then create the real secret from the example, SOPS-encrypt it, and commit.
`PLANTY_HA_URL` is deliberately empty in `configmap.yaml`, because this repository is public. Set it in the copy that lands in the Flux repo.

## What gets created

| File | What |
| --- | --- |
| `namespace.yaml` | The `planty` namespace |
| `postgres-cluster.yaml` | A single-instance CloudNativePG cluster, matching how `coop` does it |
| `deployment.yaml` | The API, plus its ClusterIP service |
| `cronjobs.yaml` | Sensor ingest, the thirst report, the daily digest, the chase, the away pass, and the cold snap watch. No watering |
| `configmap.yaml` | Non-secret configuration |
| `secret.yaml.example` | Template for the database DSN, Home Assistant token, APNs credentials, MinIO keys, and whichever credential the judge is being paid with |

Every CronJob imports the same `planty-secrets` Secret as the service. That includes the APNs key/team/private-key values used for native push delivery.

## Signing the judge in

With `PLANTY_JUDGE: "cli"` the pod runs the Claude Code binary against a subscription, so it needs a token rather than an API key:

```sh
claude setup-token          # on a machine already signed in
```

Put the result in the secret as `CLAUDE_CODE_OAUTH_TOKEN` and leave `ANTHROPIC_API_KEY` out entirely, because setting a key makes it the default.
The CLI writes state on every run, which is why the deployment and the daily CronJob mount two `emptyDir` volumes; `readOnlyRootFilesystem` stays true and nothing there is worth surviving a restart.

Photo links are signed for `PLANTY_S3_PUBLIC_ENDPOINT`, not for the endpoint the pod dials.
Leave it unset and the app is handed URLs on a cluster DNS name it cannot resolve, which looks exactly like photographs failing to upload.

## LAN only, deliberately

There is no IngressRoute in this directory, and if you add one, keep it LAN-only.
The pattern to copy is a DNS record that resolves to a private address: the name works on the local network and nowhere else.

That matters because the service has **no authentication**, and while the plant data is dull, the pod holds a long-lived Home Assistant token and a credential that can spend a Claude subscription.
Anything that makes this reachable from outside has to add real auth in the same commit.

`PLANTY_S3_PUBLIC_ENDPOINT` has to be reachable from wherever the app runs, and no further.
Give the bucket a name that resolves the same way the API's own name does, so photographs load exactly where the app already works.
Pointing it at a genuinely internet-reachable bucket would hand out presigned URLs to a photo timeline from inside a house.

## Order of operations

1. Push a tag so the release workflow builds an **arm64** image; the minis are Apple silicon and an amd64-only image will not schedule.
2. Apply the namespace, the secret, and the Postgres cluster, then wait for CNPG to report the cluster healthy.
3. Apply the deployment. It runs migrations on start, so there is no separate migration step.
4. Seed the sabbatical plants and their open questions:

   ```sh
   kubectl -n planty exec deploy/planty -- /planty seed
   ```

5. Link sensors and record calibration baselines before trusting any automated watering decision. An uncalibrated probe is not evidence.

## Nothing waters a plant on a timer

This is a standing rule, not a phase to graduate out of: **Planty reports, Joey decides.**
`planty water` is the only command that moves water and it has no CronJob, will not be given one, and is listed in `cmd/planty/main_test.go` as deliberately manual so the coverage test does not ask for one.

What is scheduled instead is `planty thirst`, twice a day, which reads every calibrated probe and says which plants are dry.
It covers hand-watered plants as well as the LetPot line, because most of them are watered by hand and those are the ones that get forgotten.
It needs no pump and no API key, so it works before any of the rest is set up.

If you ever do run the pump by hand, this is what it needs first:

1. Install `HSTEP/letpot2.0-home-assistant`, so the pump is a Home Assistant switch.
2. Turn off the LetPot app's own schedules. Two things driving one pump is how a plant gets watered twice.
3. Calibrate every probe on the line. The job refuses to run while any plant on the line lacks a calibrated sensor, which is the correct behaviour, not a bug to work around.
4. Run it by hand and watch: `kubectl -n planty exec deploy/planty -- /planty water`

It needs `PLANTY_PUMP_SWITCH`, and optionally `PLANTY_PUMP_SECONDS` (default 120).

The safety model is that the run duration is held by the process rather than by a Home Assistant `for:` trigger, and the pump-off call is deferred so a cancelled context or a crash still closes it. A `for:` trigger re-stamps `last_changed` on every restart, which is how a valve here once ran 14h33m past a 45 minute cap.

**A single wet plant vetoes the whole run.** One pump waters everything on the line, so if one plant is dry and another is already soaked, watering would drown the second. The job sends a native Planty push instead of choosing a victim.

## Before the cold snap job is useful

`planty cold` queries plants by `min_temp_f`, which the seed sets to 55F for every one of the friend's plants.
It needs the Home Assistant token only for forecast read access. Alert delivery goes through APNs and requires the APNs credentials plus at least one registered Planty device.

Run it once by hand to confirm the forecast comes back and native push is configured before relying on it:

```sh
kubectl -n planty exec deploy/planty -- /planty cold
```
