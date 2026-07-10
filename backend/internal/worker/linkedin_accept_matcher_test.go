package worker

import (
	"slices"
	"testing"

	"github.com/jcleira/magiklead/backend/internal/repository"
)

// TestMatchAcceptedLeads is issue #7's pure accept-matcher: given an account's
// current connection member ids and its awaiting_accept leads, it returns
// exactly the leads whose cached member id is now connected. Already-accepted
// leads never reach it — GetLinkedInAwaitingAcceptByAccount only returns
// awaiting rows — so the matcher's job is purely this set intersection, and
// the awaiting_accept gate in MarkLinkedInAcceptedByMember handles the rest.
func TestMatchAcceptedLeads(t *testing.T) {
	lead := func(member string) repository.GetLinkedInAwaitingAcceptByAccountRow {
		return repository.GetLinkedInAwaitingAcceptByAccountRow{MemberID: member}
	}
	set := func(ids ...string) map[string]struct{} {
		m := make(map[string]struct{}, len(ids))
		for _, id := range ids {
			m[id] = struct{}{}
		}
		return m
	}

	cases := []struct {
		name        string
		connections map[string]struct{}
		awaiting    []repository.GetLinkedInAwaitingAcceptByAccountRow
		want        []string // member ids expected to flip, in input order
	}{
		{
			name:        "match: a connected member flips",
			connections: set("ACoAA1"),
			awaiting:    []repository.GetLinkedInAwaitingAcceptByAccountRow{lead("ACoAA1")},
			want:        []string{"ACoAA1"},
		},
		{
			name:        "no-match: an unconnected member does not flip",
			connections: set("ACoAA_stranger"),
			awaiting:    []repository.GetLinkedInAwaitingAcceptByAccountRow{lead("ACoAA1")},
			want:        nil,
		},
		{
			name:        "mixed: only the connected members flip",
			connections: set("ACoAA1", "ACoAA3", "ACoAA_stranger"),
			awaiting:    []repository.GetLinkedInAwaitingAcceptByAccountRow{lead("ACoAA1"), lead("ACoAA2"), lead("ACoAA3")},
			want:        []string{"ACoAA1", "ACoAA3"},
		},
		{
			name:        "extra connections with no awaiting lead are ignored",
			connections: set("ACoAA1", "ACoAA2", "ACoAA3"),
			awaiting:    []repository.GetLinkedInAwaitingAcceptByAccountRow{lead("ACoAA2")},
			want:        []string{"ACoAA2"},
		},
		{
			name:        "empty connections: nothing flips",
			connections: set(),
			awaiting:    []repository.GetLinkedInAwaitingAcceptByAccountRow{lead("ACoAA1")},
			want:        nil,
		},
		{
			// An already-accepted lead is no longer awaiting_accept, so the
			// upstream query omits it — the matcher simply never sees it.
			name:        "no awaiting leads: nothing flips (already-accepted are absent upstream)",
			connections: set("ACoAA1"),
			awaiting:    nil,
			want:        nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := matchAcceptedLeads(tc.connections, tc.awaiting)
			gotIDs := make([]string, len(got))
			for i, l := range got {
				gotIDs[i] = l.MemberID
			}
			if !slices.Equal(gotIDs, tc.want) {
				t.Errorf("matchAcceptedLeads = %v, want %v", gotIDs, tc.want)
			}
		})
	}
}
