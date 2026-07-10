-- name: ListClientTrainersByTelegramUser :many
SELECT
  tc.trainer_id,
  tu.display_name AS trainer_display_name,
  pa.id AS assignment_id,
  pv.name AS program_name,
  pv.name_ru AS program_name_ru
FROM mentorix.auth_identities ai
INNER JOIN mentorix.trainer_clients tc
  ON tc.client_user_id = ai.user_id
  AND tc.status = 'active'
INNER JOIN mentorix.trainers t ON t.id = tc.trainer_id
INNER JOIN mentorix.users tu ON tu.id = t.user_id
LEFT JOIN mentorix.program_assignments pa
  ON pa.trainer_id = tc.trainer_id
  AND pa.client_user_id = tc.client_user_id
  AND pa.status = 'active'
LEFT JOIN mentorix.program_versions pv ON pv.id = pa.program_version_id
WHERE ai.provider = $1
  AND ai.subject = $2
ORDER BY tc.created_at DESC;

-- name: GetClientUserIDByTelegram :one
SELECT user_id
FROM mentorix.auth_identities
WHERE provider = $1
  AND subject = $2;

-- name: GetTelegramSubjectByUserID :one
SELECT subject
FROM mentorix.auth_identities
WHERE user_id = $1
  AND provider = $2;

-- name: GetTrainerDisplayNameByTrainerID :one
SELECT u.display_name
FROM mentorix.trainers t
INNER JOIN mentorix.users u ON u.id = t.user_id
WHERE t.id = $1;

-- name: GetProgramVersionDisplayByID :one
SELECT name, name_ru
FROM mentorix.program_versions
WHERE id = $1;
