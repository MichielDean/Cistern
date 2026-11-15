package main

import "github.com/MichielDean/cistern/internal/cistern"

// assigneeKey identifies an aqueduct worker by (repo, worker name). Worker
// names are only unique per repo; two repos may both define "virgo".
type assigneeKey struct {
	repo string // Droplet.Repo value this assignment belongs to
	name string // bare worker name (Assignee)
}

// lookupAssignee returns the droplet assigned to (repo, name). Droplets with
// an empty Repo (legacy rows / cistern-internal droplets) match any repo with
// that worker name, after the exact repo match. Returns nil, false when
// unassigned.
//
// Precondition: m came from buildAssigneeMap — entries are keyed by
// {droplet.Repo, droplet.Assignee}.
func lookupAssignee(m map[assigneeKey]*cistern.Droplet, repo, name string) (*cistern.Droplet, bool) {
	if d, ok := m[assigneeKey{repo: repo, name: name}]; ok {
		return d, true
	}
	if d, ok := m[assigneeKey{repo: "", name: name}]; ok && repo != "" {
		return d, true
	}
	return nil, false
}

// buildAssigneeMap indexes in_progress droplets by (repo, name). Droplets with
// an empty Repo are stored under the "" repo key for fallback matching. The
// returned map is never nil.
func buildAssigneeMap(items []*cistern.Droplet) map[assigneeKey]*cistern.Droplet {
	m := make(map[assigneeKey]*cistern.Droplet)
	for _, item := range items {
		if item.Status != "in_progress" || item.Assignee == "" {
			continue
		}
		m[assigneeKey{repo: item.Repo, name: item.Assignee}] = item
	}
	return m
}
