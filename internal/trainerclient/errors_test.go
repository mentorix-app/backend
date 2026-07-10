package trainerclient_test

import (
	"testing"

	"mentorix-backend/internal/program"
	"mentorix-backend/internal/trainerclient"
)

func TestHTTPErrorFrom_inviteErrors(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{trainerclient.ErrInviteNotFound, 404},
		{trainerclient.ErrInviteExpired, 410},
		{trainerclient.ErrInviteConsumed, 409},
		{trainerclient.ErrInviteNotConfigured, 503},
		{program.ErrForbidden, 403},
		{program.ErrClientNotLinked, 403},
		{program.ErrClientBlocked, 422},
		{program.ErrClientNotFound, 404},
		{program.ErrNotFound, 404},
		{program.ErrProgramNotPublished, 422},
	}
	for _, tc := range cases {
		he := trainerclient.HTTPErrorFrom(tc.err)
		if he.Code != tc.code {
			t.Fatalf("%v: code = %d, want %d", tc.err, he.Code, tc.code)
		}
	}
}
