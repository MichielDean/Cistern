package cistern

import (
	"strings"
	"testing"
)

func mustRecordEvent(t *testing.T, c *Client, id, eventType string) {
	t.Helper()
	if err := c.RecordEvent(id, eventType, "{}"); err != nil {
		t.Fatalf("RecordEvent %s: %v", eventType, err)
	}
}

// TestSyncNotifications_PromotesQualifyingEvents verifies that stall events
// become notifications carrying the droplet title, and that bookkeeping
// events are skipped.
func TestSyncNotifications_PromotesQualifyingEvents(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	d, err := c.Add("repo-a", "Build parser", "does a thing", 2)
	if err != nil {
		t.Fatal(err)
	}
	mustRecordEvent(t, c, d.ID, EventCreate)    // not qualifying
	mustRecordEvent(t, c, d.ID, EventStall)     // qualifying
	mustRecordEvent(t, c, d.ID, EventEdit)      // not qualifying
	mustRecordEvent(t, c, d.ID, EventDelivered) // qualifying

	created, err := c.SyncNotifications()
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(created))
	}
	if created[0].Title != "Build parser: stall" || created[0].DropletID != d.ID {
		t.Errorf("unexpected first notification: %+v", created[0])
	}

	unread, err := c.UnreadNotificationCount()
	if err != nil {
		t.Fatal(err)
	}
	if unread != 2 {
		t.Errorf("expected unread count 2, got %d", unread)
	}
}

// TestSyncNotifications_Idempotent verifies that re-running the sync never
// duplicates notifications, thanks to the UNIQUE event_row_id constraint.
func TestSyncNotifications_Idempotent(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	d, err := c.Add("repo-a", "Idempotent check", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	mustRecordEvent(t, c, d.ID, EventCircuitBreaker)

	if _, err := c.SyncNotifications(); err != nil {
		t.Fatal(err)
	}
	again, err := c.SyncNotifications()
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 0 {
		t.Fatalf("expected 0 notifications on re-sync, got %d", len(again))
	}
	unread, err := c.UnreadNotificationCount()
	if err != nil {
		t.Fatal(err)
	}
	if unread != 1 {
		t.Errorf("expected unread count 1, got %d", unread)
	}
}

// TestNotifications_MarkAndList covers unread filtering, single-read marking,
// mark-all, and unknown-id behavior.
func TestNotifications_MarkAndList(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	d, err := c.Add("repo-a", "Mark test", "", 2)
	if err != nil {
		t.Fatal(err)
	}
	mustRecordEvent(t, c, d.ID, EventNoRoute)
	mustRecordEvent(t, c, d.ID, EventRecovery)
	if _, err := c.SyncNotifications(); err != nil {
		t.Fatal(err)
	}

	all, err := c.ListNotifications(false, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(all))
	}
	// Newest first ordering.
	if all[0].ID <= all[1].ID {
		t.Errorf("expected descending ids, got %d then %d", all[0].ID, all[1].ID)
	}

	unreadOnly, err := c.ListNotifications(true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(unreadOnly) != 2 {
		t.Errorf("expected 2 unread, got %d", len(unreadOnly))
	}

	if _, err := c.MarkNotificationRead(all[0].ID); err != nil {
		t.Fatalf("mark read: %v", err)
	}
	// Marking again is a no-op success.
	if _, err := c.MarkNotificationRead(all[0].ID); err != nil {
		t.Errorf("second mark read should succeed, got %v", err)
	}
	unread, err := c.UnreadNotificationCount()
	if err != nil {
		t.Fatal(err)
	}
	if unread != 1 {
		t.Errorf("expected unread count 1, got %d", unread)
	}
	unreadOnly, err = c.ListNotifications(true, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(unreadOnly) != 1 {
		t.Errorf("expected 1 unread after single read, got %d", len(unreadOnly))
	}

	marked, err := c.MarkAllNotificationsRead()
	if err != nil {
		t.Fatal(err)
	}
	if marked != 1 {
		t.Errorf("expected 1 marked, got %d", marked)
	}
	unread, err = c.UnreadNotificationCount()
	if err != nil {
		t.Fatal(err)
	}
	if unread != 0 {
		t.Errorf("expected unread count 0, got %d", unread)
	}

	// Unknown id reports not-found error text for the dashboard 404 mapping.
	_, err = c.MarkNotificationRead(999999)
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error for unknown id, got %v", err)
	}
}

// TestSyncNotifications_MissingDropletFallsBackToID verifies the LEFT JOIN
// fallback: an event for an unknown droplet still produces a notification
// titled with the droplet id.
func TestSyncNotifications_MissingDropletFallsBackToID(t *testing.T) {
	c := testClient(t)
	defer c.Close()

	mustRecordEvent(t, c, "drp-ghost", EventLoopRecovery)
	// loop_recovery is not qualifying; use a qualifying type against the ghost id.
	mustRecordEvent(t, c, "drp-ghost", EventStall)

	created, err := c.SyncNotifications()
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(created))
	}
	if created[0].Title != "drp-ghost: stall" {
		t.Errorf("expected id fallback title, got %q", created[0].Title)
	}
}
