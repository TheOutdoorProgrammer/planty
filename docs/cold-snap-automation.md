# Cold-snap watch

Planty's afternoon cold watch is shipped as the `planty cold` CronJob.
It warns before the temperature falls, records which plants were carried inside, and later tells the user when those plants can go back out.

## Why it uses a forecast

A current-temperature trigger fires after the useful action window has closed.
The job therefore reads a daily forecast in mid-afternoon, while there is still time to move pots before the overnight low.

`PLANTY_WEATHER_ENTITY` must name a Home Assistant weather entity that supports a daily forecast.
The live deployment uses `weather.forecast_home`; the public deployment template must be overridden when its example entity cannot return daily periods.

## Warning rule

Each plant may carry its own `min_temp_f`.
Planty warns when the forecast low is within 3F of that minimum, which intentionally favors the cheap false positive over the expensive false negative.
A forecast is regional, a porch can run colder, and the cost of carrying a pot unnecessarily is much smaller than the cost of killing it.

The APNs alert names the at-risk plants, their owners, and the forecast low.
When away mode is active, the backup contact is included in the text, but it is not a second delivery route.
Planty does not use Home Assistant notifications or house speakers.

## Shelter state

The warning is answerable through `POST /v1/shelter`.
Sheltering a plant sets its `sheltered_at` timestamp, prevents the same outside warning from repeating, and makes the plant eligible for the reverse workflow.
Clients may shelter selected slugs or all plants with a temperature threshold because the real interaction often happens with an armful of pots.

Once the forecast is warm enough for the most tender sheltered plant, `planty cold` sends a second APNs alert telling the user to put the plants back out.
`POST /v1/unshelter` clears the state after that happens.
Without this half of the loop, a successful cold warning could leave outdoor plants in a dark room indefinitely.

## Boundaries

Planty knows only what the forecast says and what a person recorded through shelter or unshelter.
It does not infer a plant's physical location, move anything, escalate through speakers, or repeatedly announce an unacknowledged alert.

The owner's 55F instruction remains authoritative for the sabbatical plants.
Species-specific exceptions, including whether the sequoia sprout needs the same threshold, should be confirmed with the owner and then recorded on the plant rather than hardcoded into the job.
