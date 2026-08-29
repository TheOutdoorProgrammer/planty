-- +goose Up

ALTER TABLE garden_incidents ADD COLUMN reason text;

UPDATE garden_incidents AS incident
SET reason = incident.summary || ' ' || findings.reason
FROM (
    SELECT membership.incident_id,
           string_agg(
               plant.common_name || ' was marked ' || (membership.evidence->>'action') ||
               '. Agent reason: ' || btrim(verdict.reasoning),
               ' ' ORDER BY plant.common_name
           ) AS reason
    FROM garden_incident_plants AS membership
    JOIN plants AS plant ON plant.id = membership.plant_id
    JOIN verdicts AS verdict ON verdict.id = (membership.evidence->>'verdict_id')::uuid
    WHERE length(btrim(verdict.reasoning)) > 0
    GROUP BY membership.incident_id
) AS findings
WHERE findings.incident_id = incident.id;

UPDATE garden_incidents SET reason = summary WHERE reason IS NULL;

ALTER TABLE garden_incidents
    ALTER COLUMN reason SET NOT NULL,
    ADD CONSTRAINT garden_incident_reason_required CHECK (length(btrim(reason)) > 0);

-- +goose Down

ALTER TABLE garden_incidents DROP COLUMN reason;
