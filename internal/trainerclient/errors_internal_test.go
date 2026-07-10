package trainerclient

import (
	"testing"
)

func TestHTTPErrorFrom_defaultsInternal(t *testing.T) {
	err := HTTPErrorFrom(errSentinel{})
	if err == nil || err.Code != 500 {
		t.Fatalf("err = %v", err)
	}
}

type errSentinel struct{}

func (errSentinel) Error() string { return "boom" }
