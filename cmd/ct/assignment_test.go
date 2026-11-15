package main

import (
	"testing"

	"github.com/MichielDean/cistern/internal/cistern"
)

// makeDroplet builds a cistern.Droplet for assignment tests.
func makeDroplet(id, repo, title, status, assignee string) *cistern.Droplet {
	return &cistern.Droplet{
		ID:       id,
		Repo:     repo,
		Title:    title,
		Status:   status,
		Assignee: assignee,
	}
}

// TestBuildAssigneeMap_IndexesInProgressByRepoAndName verifies that only
// in_progress droplets with a non-empty Assignee are indexed, keyed by
// (repo, assignee). Two repos with the same worker name must not collide.
func TestBuildAssigneeMap_IndexesInProgressByRepoAndName(t *testing.T) {
	tests := []struct {
		name  string
		items []*cistern.Droplet
		want  map[assigneeKey]string // key -> droplet ID
	}{
		{
			name: "unique names across repos",
			items: []*cistern.Droplet{
				makeDroplet("a1", "repoA", "Alpha", "in_progress", "virgo"),
				makeDroplet("b1", "repoB", "Beta", "in_progress", "marcia"),
			},
			want: map[assigneeKey]string{
				{repo: "repoA", name: "virgo"}:  "a1",
				{repo: "repoB", name: "marcia"}: "b1",
			},
		},
		{
			name: "colliding names across repos stay separate",
			items: []*cistern.Droplet{
				makeDroplet("a1", "repoA", "Alpha", "in_progress", "virgo"),
				makeDroplet("b1", "repoB", "Beta", "in_progress", "virgo"),
			},
			want: map[assigneeKey]string{
				{repo: "repoA", name: "virgo"}: "a1",
				{repo: "repoB", name: "virgo"}: "b1",
			},
		},
		{
			name: "empty Repo stored under empty key",
			items: []*cistern.Droplet{
				makeDroplet("c1", "", "Internal", "in_progress", "virgo"),
			},
			want: map[assigneeKey]string{
				{repo: "", name: "virgo"}: "c1",
			},
		},
		{
			name: "non-in_progress ignored",
			items: []*cistern.Droplet{
				makeDroplet("o1", "repoA", "Open", "open", "virgo"),
				makeDroplet("d1", "repoA", "Delivered", "delivered", "marcia"),
			},
			want: map[assigneeKey]string{},
		},
		{
			name: "empty Assignee ignored",
			items: []*cistern.Droplet{
				makeDroplet("u1", "repoA", "Unassigned", "in_progress", ""),
			},
			want: map[assigneeKey]string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAssigneeMap(tt.items)
			if got == nil {
				t.Fatal("buildAssigneeMap returned nil, want initialized map")
			}
			if len(got) != len(tt.want) {
				t.Fatalf("map size = %d, want %d (got %v)", len(got), len(tt.want), got)
			}
			for key, wantID := range tt.want {
				droplet, ok := got[key]
				if !ok {
					t.Errorf("missing entry for %+v", key)
					continue
				}
				if droplet.ID != wantID {
					t.Errorf("entry %+v = %q, want %q", key, droplet.ID, wantID)
				}
			}
		})
	}
}

// TestLookupAssignee_ExactMatchThenEmptyRepoFallback verifies preference
// order: exact (repo, name) first, then the ""-repo legacy fallback entry.
func TestLookupAssignee_ExactMatchThenEmptyRepoFallback(t *testing.T) {
	internal := makeDroplet("c1", "", "Internal", "in_progress", "virgo")
	repoA := makeDroplet("a1", "repoA", "Alpha", "in_progress", "virgo")

	tests := []struct {
		name     string
		m        map[assigneeKey]*cistern.Droplet
		repo     string
		lookup   string
		wantID   string
		wantBool bool
	}{
		{
			name: "exact match wins when both entries exist",
			m: map[assigneeKey]*cistern.Droplet{
				{repo: "", name: "virgo"}:      internal,
				{repo: "repoA", name: "virgo"}: repoA,
			},
			repo: "repoA", lookup: "virgo",
			wantID: "a1", wantBool: true,
		},
		{
			name: "empty-repo fallback when repo has no exact entry",
			m: map[assigneeKey]*cistern.Droplet{
				{repo: "", name: "virgo"}: internal,
			},
			repo: "repoB", lookup: "virgo",
			wantID: "c1", wantBool: true,
		},
		{
			name: "empty-repo entry matches empty repo exactly",
			m: map[assigneeKey]*cistern.Droplet{
				{repo: "", name: "virgo"}: internal,
			},
			repo: "", lookup: "virgo",
			wantID: "c1", wantBool: true,
		},
		{
			name: "miss when neither entry exists",
			m: map[assigneeKey]*cistern.Droplet{
				{repo: "repoA", name: "marcia"}: repoA,
			},
			repo: "repoA", lookup: "virgo",
			wantBool: false,
		},
		{
			name: "miss on empty map",
			m:    map[assigneeKey]*cistern.Droplet{},
			repo: "repoA", lookup: "virgo",
			wantBool: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := lookupAssignee(tt.m, tt.repo, tt.lookup)
			if ok != tt.wantBool {
				t.Fatalf("ok = %v, want %v", ok, tt.wantBool)
			}
			if tt.wantBool && got.ID != tt.wantID {
				t.Errorf("ID = %q, want %q", got.ID, tt.wantID)
			}
		})
	}
}
