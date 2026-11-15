# Cistern

Cistern routes droplets (units of work) through aqueducts of cataractae for multiple repos managed by one Castellarius.

## Language

**Worker**:
A named cataractae slot in a repo's aqueduct (e.g. `virgo`). Worker names are unique per repo — two repos may define the same name; only the (repo, name) pair is an identity.
_Avoid_: operator, cataractae (that's the step name), agent

**Worker assignment**:
The binding of an in_progress droplet to a (repo, worker) pair via `Droplet.Assignee`. Reported in dashboards, inspected by repo scope.

**Orphaned droplet**:
An in_progress droplet that no aqueduct row claims. Always a reporting artifact of a stale or colliding worker name — not a pipeline state.
_Avoid_: unassigned (kept only as the UI label for genuinely assignment-less rows)
