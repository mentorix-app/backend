-- Trainer reply on a client workout completion (one comment per completion for now).

CREATE TABLE mentorix.client_workout_completion_comments (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  client_workout_completion_id uuid NOT NULL
    REFERENCES mentorix.client_workout_completions (id) ON DELETE CASCADE,
  trainer_id uuid NOT NULL REFERENCES mentorix.trainers (id),
  comment_text text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  -- One trainer reply per completion; drop to allow a comment thread later.
  -- Name shortened to fit the 63-char identifier limit.
  CONSTRAINT client_workout_completion_comments_completion_id_uniq
    UNIQUE (client_workout_completion_id)
);

CREATE INDEX client_workout_completion_comments_trainer_id_idx
  ON mentorix.client_workout_completion_comments (trainer_id);
