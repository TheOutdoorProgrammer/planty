# Home Assistant boundary

Planty owns garden state, judgment, reminders, and notifications.
Home Assistant supplies physical-world inputs and optional actuator calls.

## What Planty reads

- Sensor states for `planty ingest`.
- A daily weather forecast for `planty cold`.
- The optional LetPot pump-state sensor used during a manual water command.

The configured long-lived token needs only the Home Assistant permissions required for those entities and the optional pump switch.
Planty does not call Home Assistant notification services or `script.announce`.

## Greenhouse airflow

Planty owns daily on/off schedules for registered semantic fans. Disable any recurring Home Assistant automation for the same entity so two controllers cannot fight. Home Assistant may retain an independent maximum-on watchdog sized beyond the intended Planty schedule window, because a physical backstop is not a second source of recurring intent.

## Mushroom care

Misting is a visual decision: mist when the surface looks dry and no water is beading on the caps.
An RH threshold cannot see that distinction and encourages over-misting, which causes bacterial blotch.

The correct automation is a Planty reminder, not a Home Assistant misting rule.
Create a `misted` reminder for the active kit, record the action when it is done, and deactivate the reminder after the final flush.
The fan schedule remains independent from mushroom misting because long thin stems indicate insufficient fresh air, not insufficient mist.

## LetPot

The LetPot DI-3 is exposed to Home Assistant through `HSTEP/letpot2.0-home-assistant`, which provides a pump switch and an active-pumping sensor.
It is cloud-dependent, so loss of internet also removes reliable actuator control.

Planty never schedules `planty water`.
Before even a manual run, disable the LetPot app's own schedules, calibrate every probe on the shared line, configure `PLANTY_PUMP_SWITCH`, and ensure the independent maximum-on safety work in [ROADMAP.md](../ROADMAP.md) is understood.
A wet plant vetoes the whole shared line.

The process currently attempts to turn the pump off when the command exits normally or is cancelled.
That cleanup cannot survive SIGKILL, OOM, node loss, or a network partition, so it is not a hardware safety boundary and must never be described as one.

## Deliberate exclusions

- No Home Assistant push-notification fallback.
- No speaker escalation.
- No RH-triggered misting.
- No scheduled moisture-triggered watering.
- No Home Assistant `for:` timer as the only maximum-on control for anything that moves water.
