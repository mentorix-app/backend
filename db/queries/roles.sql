-- name: UserHasAnyRole :one
SELECT EXISTS(
  SELECT 1 FROM mentorix.user_roles
  WHERE user_id = sqlc.arg('user_id') AND role = ANY(sqlc.arg('roles')::text[])
) AS ok;
