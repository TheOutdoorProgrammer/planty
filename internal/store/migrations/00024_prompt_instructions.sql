-- +goose Up

-- Only the user-editable overlay lives here. Safety, schema, and tool authority
-- remain in code so changing a setting cannot weaken the model boundary.
CREATE TABLE prompt_instructions (
    job          text        NOT NULL PRIMARY KEY,
    instructions text        NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT prompt_instructions_not_blank CHECK (length(btrim(instructions)) > 0)
);

-- +goose Down

DROP TABLE prompt_instructions;
