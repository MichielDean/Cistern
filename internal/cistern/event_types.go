package cistern

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	EventCreate         = "create"
	EventDispatch       = "dispatch"
	EventPass           = "pass"
	EventRecirculate    = "recirculate"
	EventDelivered      = "delivered"
	EventRestart        = "restart"
	EventApprove        = "approve"
	EventEdit           = "edit"
	EventPool           = "pool"
	EventCancel         = "cancel"
	EventExitNoOutcome  = "exit_no_outcome"
	EventStall          = "stall"
	EventRecovery       = "recovery"
	EventCircuitBreaker = "circuit_breaker"
	EventLoopRecovery   = "loop_recovery"
	EventAutoPromote    = "auto_promote"
	EventNoRoute        = "no_route"
)

var ValidEventTypes = map[string]bool{
	EventCreate:         true,
	EventDispatch:       true,
	EventPass:           true,
	EventRecirculate:    true,
	EventDelivered:      true,
	EventRestart:        true,
	EventApprove:        true,
	EventEdit:           true,
	EventPool:           true,
	EventCancel:         true,
	EventExitNoOutcome:  true,
	EventStall:          true,
	EventRecovery:       true,
	EventCircuitBreaker: true,
	EventLoopRecovery:   true,
	EventAutoPromote:    true,
	EventNoRoute:        true,
}

func parsePayload(payload string) map[string]any {
	if payload == "" || payload == "{}" {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(payload), &m); err != nil {
		return nil
	}
	return m
}

// DisplayInfo maps an event_type and its JSON payload to human-readable
// eventLabel and detail strings. This replaces the remapEvent and
// remapPayload* functions that were previously in cmd/ct/droplet_log.go.
func DisplayInfo(eventType, payload string) (eventLabel, detail string) {
	m := parsePayload(payload)

	switch eventType {
	case EventCreate:
		return "created", displayInfoCreate(m)
	case EventDispatch:
		return "dispatched", displayInfoDispatch(m)
	case EventPass:
		return "pass", displayInfoCataractaeNotes(m)
	case EventRecirculate:
		return "recirculate", displayInfoRecirculate(m)
	case EventDelivered:
		return "delivered", ""
	case EventRestart:
		return "restart", displayInfoCataractae(m)
	case EventApprove:
		return "approved", displayInfoCataractae(m)
	case EventEdit:
		return "edit", displayInfoEdit(m)
	case EventPool:
		return "pooled", displayInfoReason(m)
	case EventCancel:
		return "cancelled", displayInfoReason(m)
	case EventExitNoOutcome:
		return "exit_no_outcome", displayInfoExitNoOutcome(m)
	case EventStall:
		return "stall", displayInfoStall(m)
	case EventRecovery:
		return "recovery", displayInfoCataractae(m)
	case EventCircuitBreaker:
		return "circuit_breaker", displayInfoCircuitBreaker(m)
	case EventLoopRecovery:
		return "loop_recovery", displayInfoLoopRecovery(m)
	case EventAutoPromote:
		return "auto_promote", displayInfoAutoPromote(m)
	case EventNoRoute:
		return "no_route", displayInfoCataractae(m)
	default:
		return eventType, payload
	}
}

func displayInfoCreate(m map[string]any) string {
	if m == nil {
		return ""
	}
	var parts []string
	if repo, ok := m["repo"]; ok && repo != "" {
		parts = append(parts, fmt.Sprintf("repo: %v", repo))
	}
	if title, ok := m["title"]; ok && title != "" {
		parts = append(parts, fmt.Sprintf("title: %v", title))
	}
	if priority, ok := m["priority"]; ok {
		parts = append(parts, fmt.Sprintf("priority: %v", priority))
	}
	if complexity, ok := m["complexity"]; ok {
		parts = append(parts, fmt.Sprintf("complexity: %v", complexity))
	}
	return strings.Join(parts, ", ")
}

func displayInfoDispatch(m map[string]any) string {
	if m == nil {
		return ""
	}
	var parts []string
	if aqueduct, ok := m["aqueduct"]; ok && aqueduct != "" {
		parts = append(parts, fmt.Sprintf("aqueduct: %v", aqueduct))
	}
	if cat, ok := m["cataractae"]; ok && cat != "" {
		parts = append(parts, fmt.Sprintf("step: %v", cat))
	}
	if assignee, ok := m["assignee"]; ok && assignee != "" {
		parts = append(parts, fmt.Sprintf("assignee: %v", assignee))
	}
	return strings.Join(parts, ", ")
}

func displayInfoCataractaeNotes(m map[string]any) string {
	if m == nil {
		return ""
	}
	var parts []string
	if cat, ok := m["cataractae"]; ok && cat != "" {
		parts = append(parts, fmt.Sprintf("by: %v", cat))
	}
	if notes, ok := m["notes"]; ok && notes != "" {
		parts = append(parts, fmt.Sprintf("notes: %v", notes))
	}
	return strings.Join(parts, ", ")
}

func displayInfoRecirculate(m map[string]any) string {
	if m == nil {
		return ""
	}
	var parts []string
	if cat, ok := m["cataractae"]; ok && cat != "" {
		parts = append(parts, fmt.Sprintf("by: %v", cat))
	}
	if target, ok := m["target"]; ok && target != "" {
		parts = append(parts, fmt.Sprintf("to: %v", target))
	}
	if notes, ok := m["notes"]; ok && notes != "" {
		parts = append(parts, fmt.Sprintf("notes: %v", notes))
	}
	return strings.Join(parts, ", ")
}

func displayInfoCataractae(m map[string]any) string {
	if m == nil {
		return ""
	}
	if cat, ok := m["cataractae"]; ok && cat != "" {
		return fmt.Sprintf("by: %v", cat)
	}
	return ""
}

func displayInfoEdit(m map[string]any) string {
	if m == nil {
		return ""
	}
	if fields, ok := m["fields"]; ok {
		return fmt.Sprintf("fields: %v", fields)
	}
	return ""
}

func displayInfoReason(m map[string]any) string {
	if m == nil {
		return ""
	}
	if reason, ok := m["reason"]; ok && reason != "" {
		return fmt.Sprintf("reason: %v", reason)
	}
	return ""
}

func displayInfoExitNoOutcome(m map[string]any) string {
	if m == nil {
		return ""
	}
	var parts []string
	if session, ok := m["session"]; ok && session != "" {
		parts = append(parts, fmt.Sprintf("session: %v", session))
	}
	if worker, ok := m["worker"]; ok && worker != "" {
		parts = append(parts, fmt.Sprintf("worker: %v", worker))
	}
	if cat, ok := m["cataractae"]; ok && cat != "" {
		parts = append(parts, fmt.Sprintf("step: %v", cat))
	}
	return strings.Join(parts, ", ")
}

func displayInfoStall(m map[string]any) string {
	if m == nil {
		return ""
	}
	var parts []string
	if cat, ok := m["cataractae"]; ok && cat != "" {
		parts = append(parts, fmt.Sprintf("step: %v", cat))
	}
	if elapsed, ok := m["elapsed"]; ok && elapsed != "" {
		parts = append(parts, fmt.Sprintf("elapsed: %v", elapsed))
	}
	if heartbeat, ok := m["heartbeat"]; ok && heartbeat != "" {
		parts = append(parts, fmt.Sprintf("heartbeat: %v", heartbeat))
	}
	return strings.Join(parts, ", ")
}

func displayInfoCircuitBreaker(m map[string]any) string {
	if m == nil {
		return ""
	}
	var parts []string
	if dc, ok := m["death_count"]; ok {
		parts = append(parts, fmt.Sprintf("dead sessions: %v", dc))
	}
	if window, ok := m["window"]; ok && window != "" {
		parts = append(parts, fmt.Sprintf("window: %v", window))
	}
	return strings.Join(parts, ", ")
}

func displayInfoLoopRecovery(m map[string]any) string {
	if m == nil {
		return ""
	}
	var parts []string
	if from, ok := m["from"]; ok && from != "" {
		parts = append(parts, fmt.Sprintf("from: %v", from))
	}
	if to, ok := m["to"]; ok && to != "" {
		parts = append(parts, fmt.Sprintf("to: %v", to))
	}
	if issue, ok := m["issue"]; ok && issue != "" {
		parts = append(parts, fmt.Sprintf("issue: %v", issue))
	}
	return strings.Join(parts, ", ")
}

func displayInfoAutoPromote(m map[string]any) string {
	if m == nil {
		return ""
	}
	var parts []string
	if cat, ok := m["cataractae"]; ok && cat != "" {
		parts = append(parts, fmt.Sprintf("step: %v", cat))
	}
	if routedTo, ok := m["routed_to"]; ok && routedTo != "" {
		parts = append(parts, fmt.Sprintf("routed to: %v", routedTo))
	}
	return strings.Join(parts, ", ")
}
