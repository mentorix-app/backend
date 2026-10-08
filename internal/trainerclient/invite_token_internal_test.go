package trainerclient

import (
	"errors"
	"testing"
)

func TestInviteTokenFromInput(t *testing.T) {
	const tok = "Ab_-09xYzQ1234567890ab"
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "bare token", input: tok, want: tok},
		{name: "inv prefix", input: "inv_" + tok, want: tok},
		{name: "full link", input: "https://t.me/somebot?start=inv_" + tok, want: tok},
		{name: "full link with trailing param", input: "https://t.me/somebot?start=inv_" + tok + "&x=1", want: tok},
		{name: "start param without prefix", input: "https://t.me/somebot?start=" + tok, want: tok},
		{name: "link followed by text", input: "join: https://t.me/somebot?start=inv_" + tok + " thanks", want: tok},
		{name: "surrounding spaces", input: "  \n" + tok + "\t ", want: tok},
		{name: "empty", input: "", wantErr: true},
		{name: "blank", input: "   ", wantErr: true},
		{name: "only prefix", input: "inv_", wantErr: true},
		{name: "empty start value", input: "https://t.me/somebot?start=&x=1", wantErr: true},
		{name: "garbage with punctuation", input: "not a token!", wantErr: true},
		{name: "sql chars", input: "x'; DROP TABLE users;--", wantErr: true},
		{name: "unicode", input: "токен", wantErr: true},
		{name: "emoji", input: "inv_\U0001F600", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := inviteTokenFromInput(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrInviteNotFound) {
					t.Fatalf("err = %v, want ErrInviteNotFound", err)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("got %q, %v; want %q", got, err, tt.want)
			}
		})
	}
}

func TestInviteTokenFromInput_acceptsGeneratedTokens(t *testing.T) {
	for i := 0; i < 200; i++ {
		tok, err := newInviteToken()
		if err != nil {
			t.Fatal(err)
		}
		got, err := inviteTokenFromInput(inviteURL("bot", tok))
		if err != nil || got != tok {
			t.Fatalf("link of %q gave %q, %v", tok, got, err)
		}
	}
}
