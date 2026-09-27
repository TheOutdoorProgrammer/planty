-- +goose Up
ALTER TABLE judgment_results DROP CONSTRAINT judgment_results_attempts_check;
ALTER TABLE judgment_results ADD CONSTRAINT judgment_results_attempts_check
    CHECK (attempts >= 0 AND (NOT succeeded OR attempts > 0));

-- +goose Down
UPDATE judgment_results SET attempts = 1 WHERE attempts = 0;
ALTER TABLE judgment_results DROP CONSTRAINT judgment_results_attempts_check;
ALTER TABLE judgment_results ADD CONSTRAINT judgment_results_attempts_check CHECK (attempts > 0);
