package http

const (
	MsgInternal               = "internal"
	MsgUnauthorized           = "unauthorized"
	MsgInvalidJSON            = "invalid json"
	MsgInvalidID              = "invalid id"
	MsgInvalidUserID          = "invalid user_id"
	MsgUserNotFound           = "user not found"
	MsgExerciseNotFound       = "exercise not found"
	MsgProgramNotFound        = "program not found"
	MsgForbidden              = "forbidden"
	MsgInvalidToken           = "invalid token"
	MsgMissingAuth            = "missing authorization"
	MsgMissingBearerToken     = "missing bearer token"
	MsgTrainerRoleRequired    = "trainer role required"
	MsgAdminRoleRequired      = "admin role required"
)
