package program

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// TestApplyBlockClients_noRulesYieldsEmptyArray pins that a block with no
// visibility rules gets an empty, non-nil ClientUserIDs slice, so the field
// always marshals as [] and never as null: a map miss on rules must not leak
// through as a nil slice.
func TestApplyBlockClients_noRulesYieldsEmptyArray(t *testing.T) {
	blockKey := uuid.New()
	d := Detail{
		Weeks: []Week{
			{Days: []Day{
				{Blocks: []DayBlock{
					{BlockKey: blockKey},
				}},
			}},
		},
	}

	applyBlockClients(&d, map[uuid.UUID][]uuid.UUID{})

	block := d.Weeks[0].Days[0].Blocks[0]
	if block.ClientUserIDs == nil {
		t.Fatal("ClientUserIDs is nil, want empty non-nil slice")
	}
	if len(block.ClientUserIDs) != 0 {
		t.Fatalf("ClientUserIDs = %v, want empty", block.ClientUserIDs)
	}

	b, err := json.Marshal(block)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"client_user_ids":[]`) {
		t.Fatalf("marshaled block = %s, want client_user_ids:[]", b)
	}
}

// TestApplyBlockClients_withRulesKeepsClientList checks the non-empty case
// still round-trips the rule's client ids untouched.
func TestApplyBlockClients_withRulesKeepsClientList(t *testing.T) {
	blockKey := uuid.New()
	client := uuid.New()
	d := Detail{
		Weeks: []Week{
			{Days: []Day{
				{Blocks: []DayBlock{
					{BlockKey: blockKey},
				}},
			}},
		},
	}

	applyBlockClients(&d, map[uuid.UUID][]uuid.UUID{blockKey: {client}})

	got := d.Weeks[0].Days[0].Blocks[0].ClientUserIDs
	if len(got) != 1 || got[0] != client {
		t.Fatalf("ClientUserIDs = %v, want [%v]", got, client)
	}
}
