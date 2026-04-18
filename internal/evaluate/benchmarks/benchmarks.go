package benchmarks

// Deprecated: The benchmark subcommand now scores real merged PRs via
// ct evaluate benchmark --pr/--merged-since instead of synthetic items.
// This package is retained for reference but is not used by the CLI.

// Item is a synthetic work item designed to exercise specific rubric dimensions.
// Each item represents a real-world task type that the Cistern pipeline handles.
type Item struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Complexity  string `json:"complexity"`          // "standard", "full", "critical"
	Description string `json:"description"`
	// Exercises lists which rubric dimensions this item is designed to stress.
	Exercises []string `json:"exercises"`
}

// DefaultItems returns the canonical benchmark suite.
func DefaultItems() []Item {
	return []Item{
		{
			ID:         "bench-001",
			Title:      "Replace boolean columns with catalog/assignment tables",
			Complexity:  "full",
			Description: `Replace 13 boolean columns on the organization table with a scalable
two-table system: an organization_permission catalog table and an
organization_permission_assignment join table. Each boolean becomes a
row in the catalog. Query columns must support SELECT projection and
WHERE filtering. The existing search validator must be updated. Mapping
functions must transform the new structure into domain objects. Migrations
must be safe and reversible. Integration tests must cover the new query
paths. Constants must live in their own object, not in the table definition.
The permission column class must be reusable for any entity, not coupled
to Organization. Error messages for missing catalog entries must be
actionable. Boolean extraction for mapping must use a DRY helper, not
repeated inline expressions.`,
			Exercises: []string{
				"contract_correctness",
				"integration_coverage",
				"coupling",
				"migration_safety",
				"idiom_fit",
				"dry",
				"naming_clarity",
				"error_messages",
			},
		},
		{
			ID:         "bench-002",
			Title:      "Add audit trail with typed event store",
			Complexity:  "full",
			Description: `Add an audit trail system using a typed event store pattern. Create an
audit_event table with a JSON payload column and a discriminator column
for event type. Add a DAO that queries events by type, entity, and time
range. Create query column classes that project audit data in search
results. Must support filtering by event type and date range in the
existing search framework. The event type must be a sealed class hierarchy
with per-type deserialization, not a raw string. Migrations must separate
DDL from seed data. Integration tests must cover the new query paths.
The audit DAO must not couple to any specific entity type.`,
			Exercises: []string{
				"contract_correctness",
				"integration_coverage",
				"coupling",
				"migration_safety",
				"idiom_fit",
				"naming_clarity",
			},
		},
		{
			ID:         "bench-003",
			Title:      "Implement rate limiting middleware with Redis sliding window",
			Complexity:  "standard",
			Description: `Implement rate limiting middleware for the API using a Redis sliding
window algorithm. Create a RateLimiter interface with a Redis
implementation and an in-memory implementation for testing. The middleware
must read the rate limit configuration from a YAML config file. Return
429 with a Retry-After header when the limit is exceeded. The Redis key
format must be documented in code comments. Unit tests must cover both
implementations. Error handling must distinguish between Redis connection
failures (fail-open) and rate limit exceeded (fail-closed). No migrations
required. The rate limiter must not import any specific HTTP framework
type — it must be framework-agnostic.`,
			Exercises: []string{
				"contract_correctness",
				"idiom_fit",
				"naming_clarity",
				"error_messages",
			},
		},
		{
			ID:         "bench-004",
			Title:      "Migrate user preferences from JSON blob to typed columns",
			Complexity:  "full",
			Description: `The user table has a JSON blob column called preferences that stores
an unstructured key-value map. Migrate this to a two-table system: a
user_preference_catalog table defining valid preference keys with types,
and a user_preference_assignment table for per-user values. Add a DAO
that loads preferences for a list of users in a single query (not N+1).
Create a query column class that projects preference values in search.
The preference catalog must be seeded via migration with meaningful
descriptions. Preferences must use the semantically correct collection
type (Set for unique preference keys). Error messages for missing
catalog entries must name the missing key. Migrations must backtick-quote
identifiers. The preference loading function must not hardcode the User
table — make it work for any entity with preferences.`,
			Exercises: []string{
				"contract_correctness",
				"integration_coverage",
				"coupling",
				"migration_safety",
				"idiom_fit",
				"dry",
				"naming_clarity",
				"error_messages",
			},
		},
		{
			ID:         "bench-005",
			Title:      "Add webhook delivery with retry and dead letter queue",
			Complexity:  "critical",
			Description: `Add a webhook delivery system that queues outgoing HTTP requests and
retries on failure with exponential backoff. Create a webhook table,
a webhook_delivery table for tracking attempts, and a webhook_dead_letter
table for permanently failed deliveries. Add a background worker that
polls for pending deliveries and executes them. Retry up to 3 times with
backoff. After max retries, move to dead letter queue. Add a DAO that
queries delivery status by webhook and time range. Create query column
classes that project webhook status in search. Migrations must separate
DDL from indexes. The webhook executor must not couple to any specific
HTTP client library. Error messages must include the webhook ID and
attempt number. Integration tests must cover the retry and dead letter
paths with a real HTTP server stub.`,
			Exercises: []string{
				"contract_correctness",
				"integration_coverage",
				"coupling",
				"migration_safety",
				"naming_clarity",
				"error_messages",
			},
		},
	}
}