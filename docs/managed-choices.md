# Managed household choices

Planty keeps protocol concepts such as plant domain, watering method, status, and sensor role as enums. Household vocabulary is different: rooms, people, sensor places, and pot materials belong to the user and must remain editable without a schema migration.

`GET /v1/choices` returns three managed lists:

- `places` — merged from plant `location`, plant `ha_area`, sensor-link `zone`, and live Home Assistant areas
- `owners` — values already used as plant stewards, plus the built-in `self` value
- `pot_materials` — values already used on plants

Each list has `recent` and `all`. Entries include the display `value`, contributing `sources`, and `last_used_at` when the value has been used in Planty's database. Home Assistant is enrichment only: if it is unavailable, the endpoint still returns Planty's stored choices.

Place identity ignores case and punctuation so values such as `Living Room`, `living-room`, and `living_room` collapse into one choice. Planty does not guess semantic synonyms such as `Living Room` and `Lounge`.

Clients should present the lists as searchable choices and always retain an explicit custom-value action. A custom value becomes part of the catalog naturally after the record using it is saved. No separate taxonomy table or database enum is required.

For new plants, one selected Place is written to both `location` and `ha_area`. Existing plants with divergent legacy values are not rewritten by unrelated edits; explicitly changing Place converges both fields. Ambient sensor links continue to use the existing `zone` wire field, but the UI selects that value from the same Place catalog.
