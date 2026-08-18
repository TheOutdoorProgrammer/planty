# Deploying Planty

These manifests are **not** applied.
Flux reconciles whatever lands in the `flux` repo, so committing them there is the same act as deploying.
They live here until you have read them.

## Moving them into Flux

```sh
FLUX=~/projects/src/github.com/TheOutdoorProgrammer/flux/clusters/mini-2/planty
mkdir -p "$FLUX"
cp deploy/*.yaml "$FLUX"/
rm "$FLUX"/secret.yaml.example      # the example never belongs in the cluster
```

Then create the real secret from the example, SOPS-encrypt it, and commit.

## What gets created

| File | What |
| --- | --- |
| `namespace.yaml` | The `planty` namespace |
| `postgres-cluster.yaml` | A single-instance CloudNativePG cluster, matching how `coop` does it |
| `deployment.yaml` | The API, plus its ClusterIP service |
| `cronjobs.yaml` | Sensor ingest, the daily digest, and the cold snap watch |
| `configmap.yaml` | Non-secret configuration |
| `secret.yaml.example` | Template for the database DSN, the Home Assistant token, and the Anthropic key |

## No ingress, deliberately

There is **no IngressRoute and no external DNS entry here**, and that is not an oversight.
The service has no authentication, and while the plant data is dull, the pod holds a long-lived Home Assistant token and an Anthropic API key.
It stays reachable only from inside the cluster and the LAN.

The iOS app reaches it over the LAN.
If that ever needs to change, put real authentication in front of it first.

## Order of operations

1. Push a tag so the release workflow builds an **arm64** image; the minis are Apple silicon and an amd64-only image will not schedule.
2. Apply the namespace, the secret, and the Postgres cluster, then wait for CNPG to report the cluster healthy.
3. Apply the deployment. It runs migrations on start, so there is no separate migration step.
4. Seed the sabbatical plants and their open questions:

   ```sh
   kubectl -n planty exec deploy/planty -- /planty seed
   ```

5. Link sensors and record calibration baselines before trusting any automated watering decision. An uncalibrated probe is not evidence.

## `planty water` has no CronJob, on purpose

Every other job reads and notifies. This one moves water, so it does not get scheduled until you have decided it should be.

Before enabling it:

1. Install `HSTEP/letpot2.0-home-assistant`, so the pump is a Home Assistant switch.
2. Turn off the LetPot app's own schedules. Two things driving one pump is how a plant gets watered twice.
3. Calibrate every probe on the line. The job refuses to run while any plant on the line lacks a calibrated sensor, which is the correct behaviour, not a bug to work around.
4. Run it by hand and watch: `kubectl -n planty exec deploy/planty -- /planty water`

It needs `PLANTY_PUMP_SWITCH`, and optionally `PLANTY_PUMP_SECONDS` (default 120).

The safety model is that the run duration is held by the process rather than by a Home Assistant `for:` trigger, and the pump-off call is deferred so a cancelled context or a crash still closes it. A `for:` trigger re-stamps `last_changed` on every restart, which is how a valve here once ran 14h33m past a 45 minute cap.

**A single wet plant vetoes the whole run.** One pump waters everything on the line, so if one plant is dry and another is already soaked, watering would drown the second. The job notifies instead of choosing a victim.

## Before the cold snap job is useful

`planty cold` queries plants by `min_temp_f`, which the seed sets to 55F for every one of the friend's plants.
It needs the Home Assistant token to have forecast read access and permission to call `notify`.

Run it once by hand to confirm the forecast comes back before relying on it:

```sh
kubectl -n planty exec deploy/planty -- /planty cold
```
