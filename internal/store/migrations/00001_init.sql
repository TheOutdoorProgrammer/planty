-- +goose Up

CREATE TYPE plant_domain AS ENUM ('houseplant', 'edible_indoor', 'edible_outdoor');
CREATE TYPE plant_status AS ENUM ('alive', 'struggling', 'dormant', 'dead', 'gone');
CREATE TYPE watering_method AS ENUM ('letpot', 'hand');
CREATE TYPE accessibility AS ENUM ('easy', 'awkward', 'hard');
CREATE TYPE light_exposure AS ENUM ('direct', 'bright_indirect', 'medium', 'low');

CREATE TABLE plants (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            text NOT NULL UNIQUE,

    common_name     text NOT NULL,
    botanical_name  text,
    variety         text,

    domain          plant_domain    NOT NULL,
    steward         text            NOT NULL DEFAULT 'self',
    status          plant_status    NOT NULL DEFAULT 'alive',

    location        text NOT NULL DEFAULT '',
    ha_area         text,

    accessibility   accessibility   NOT NULL DEFAULT 'easy',
    watering_method watering_method NOT NULL DEFAULT 'hand',
    letpot_dripper  int,

    pot_size_in     numeric,
    pot_material    text,
    has_drainage    boolean,
    soil_mix        text,
    light_exposure  light_exposure,

    min_temp_f      numeric,
    care_profile    jsonb NOT NULL DEFAULT '{}'::jsonb,

    acquired_at     date,
    archived_at     timestamptz,

    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    -- A dripper number only means something on the LetPot line.
    CONSTRAINT dripper_needs_letpot CHECK (
        letpot_dripper IS NULL OR watering_method = 'letpot'
    )
);

-- The cold-snap job runs nightly and asks only this question.
CREATE INDEX plants_cold_watch ON plants (min_temp_f)
    WHERE archived_at IS NULL AND min_temp_f IS NOT NULL;

CREATE INDEX plants_active ON plants (domain, status) WHERE archived_at IS NULL;
CREATE INDEX plants_steward ON plants (steward) WHERE archived_at IS NULL;

CREATE TYPE sensor_role AS ENUM (
    'soil_moisture', 'ambient_temp', 'ambient_humidity', 'illuminance'
);

CREATE TABLE sensor_links (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id      uuid REFERENCES plants (id) ON DELETE CASCADE,
    zone          text,

    ha_entity_id  text        NOT NULL,
    role          sensor_role NOT NULL,

    dry_baseline  numeric,
    wet_baseline  numeric,
    calibrated_at timestamptz,

    created_at    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT link_has_a_subject CHECK (plant_id IS NOT NULL OR zone IS NOT NULL),
    CONSTRAINT soil_belongs_to_a_plant CHECK (
        role <> 'soil_moisture' OR plant_id IS NOT NULL
    ),
    UNIQUE (ha_entity_id, role)
);

CREATE INDEX sensor_links_plant ON sensor_links (plant_id);

CREATE TABLE readings (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    sensor_link_id uuid        NOT NULL REFERENCES sensor_links (id) ON DELETE CASCADE,
    value          numeric     NOT NULL,
    unit           text,
    taken_at       timestamptz NOT NULL
);

CREATE INDEX readings_link_time ON readings (sensor_link_id, taken_at DESC);

CREATE TYPE observation_kind AS ENUM (
    'watered', 'repotted', 'fertilized', 'pruned',
    'harvested', 'moved', 'symptom', 'note', 'died'
);
CREATE TYPE observation_source AS ENUM ('app', 'agent', 'automation');

CREATE TABLE observations (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id    uuid               NOT NULL REFERENCES plants (id) ON DELETE CASCADE,
    kind        observation_kind   NOT NULL,
    body        text               NOT NULL DEFAULT '',
    occurred_at timestamptz        NOT NULL,
    source      observation_source NOT NULL,
    actor       text,
    created_at  timestamptz        NOT NULL DEFAULT now()
);

CREATE INDEX observations_plant_time ON observations (plant_id, occurred_at DESC);

CREATE TABLE photos (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id        uuid        NOT NULL REFERENCES plants (id) ON DELETE CASCADE,
    storage_key     text        NOT NULL UNIQUE,
    taken_at        timestamptz NOT NULL,
    caption         text,
    vision_findings text,
    analyzed_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX photos_plant_time ON photos (plant_id, taken_at DESC);

CREATE TYPE verdict_action AS ENUM ('none', 'water', 'check', 'urgent', 'harvest');

CREATE TABLE verdicts (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id        uuid           NOT NULL REFERENCES plants (id) ON DELETE CASCADE,
    for_date        date           NOT NULL,
    action          verdict_action NOT NULL,
    reasoning       text           NOT NULL DEFAULT '',
    confidence      numeric        NOT NULL DEFAULT 0,
    evidence        jsonb          NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz    NOT NULL DEFAULT now(),
    acknowledged_at timestamptz,

    UNIQUE (plant_id, for_date)
);

CREATE INDEX verdicts_open ON verdicts (for_date DESC)
    WHERE acknowledged_at IS NULL AND action <> 'none';

CREATE TABLE harvests (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plant_id    uuid        NOT NULL REFERENCES plants (id) ON DELETE CASCADE,
    occurred_at timestamptz NOT NULL,
    quantity    numeric     NOT NULL,
    unit        text        NOT NULL,
    notes       text,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX harvests_plant_time ON harvests (plant_id, occurred_at DESC);

-- +goose Down

DROP TABLE harvests;
DROP TABLE verdicts;
DROP TYPE verdict_action;
DROP TABLE photos;
DROP TABLE observations;
DROP TYPE observation_source;
DROP TYPE observation_kind;
DROP TABLE readings;
DROP TABLE sensor_links;
DROP TYPE sensor_role;
DROP TABLE plants;
DROP TYPE light_exposure;
DROP TYPE accessibility;
DROP TYPE watering_method;
DROP TYPE plant_status;
DROP TYPE plant_domain;
