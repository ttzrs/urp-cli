// Package browser - Recorder persists browser sessions to Memgraph.
package browser

import (
	"context"
	"fmt"
	"time"

	"github.com/joss/urp/internal/graph"
)

// GraphRecorder persists browser events to Memgraph.
type GraphRecorder struct {
	db graph.Driver
}

// NewGraphRecorder creates a recorder with Memgraph connection.
func NewGraphRecorder(db graph.Driver) *GraphRecorder {
	return &GraphRecorder{db: db}
}

// RecordEvent persists a single event to the graph.
func (r *GraphRecorder) RecordEvent(ctx context.Context, sessionID string, event Event) error {
	query := `
		MATCH (s:BrowserSession {id: $sessionId})
		CREATE (e:BrowserEvent {
			type: $type,
			timestamp: $timestamp,
			url: $url,
			selector: $selector,
			value: $value,
			posX: $posX,
			posY: $posY
		})
		CREATE (s)-[:HAS_EVENT]->(e)
	`

	posX, posY := 0.0, 0.0
	if event.Position != nil {
		posX, posY = event.Position.X, event.Position.Y
	}

	return r.db.ExecuteWrite(ctx, query, map[string]any{
		"sessionId": sessionID,
		"type":      event.Type,
		"timestamp": event.Timestamp.UnixMilli(),
		"url":       event.URL,
		"selector":  event.Selector,
		"value":     event.Value,
		"posX":      posX,
		"posY":      posY,
	})
}

// SaveSession persists a complete session to the graph.
func (r *GraphRecorder) SaveSession(ctx context.Context, session *Session) error {
	// Create session node
	createSession := `
		CREATE (s:BrowserSession {
			id: $id,
			startUrl: $startUrl,
			startedAt: $startedAt,
			endedAt: $endedAt,
			eventCount: $eventCount
		})
	`

	endedAt := int64(0)
	if !session.EndedAt.IsZero() {
		endedAt = session.EndedAt.UnixMilli()
	}

	if err := r.db.ExecuteWrite(ctx, createSession, map[string]any{
		"id":         session.ID,
		"startUrl":   session.StartURL,
		"startedAt":  session.StartedAt.UnixMilli(),
		"endedAt":    endedAt,
		"eventCount": len(session.Events),
	}); err != nil {
		return fmt.Errorf("create session: %w", err)
	}

	// Create event nodes in batch
	if len(session.Events) > 0 {
		if err := r.batchCreateEvents(ctx, session.ID, session.Events); err != nil {
			return fmt.Errorf("create events: %w", err)
		}
	}

	// Create flow patterns from events
	if err := r.analyzeFlow(ctx, session); err != nil {
		return fmt.Errorf("analyze flow: %w", err)
	}

	return nil
}

// batchCreateEvents creates multiple events efficiently.
func (r *GraphRecorder) batchCreateEvents(ctx context.Context, sessionID string, events []Event) error {
	// Convert events to map format for UNWIND
	eventMaps := make([]map[string]any, len(events))
	for i, e := range events {
		posX, posY := 0.0, 0.0
		if e.Position != nil {
			posX, posY = e.Position.X, e.Position.Y
		}

		eventMaps[i] = map[string]any{
			"type":      e.Type,
			"timestamp": e.Timestamp.UnixMilli(),
			"url":       e.URL,
			"selector":  e.Selector,
			"value":     e.Value,
			"posX":      posX,
			"posY":      posY,
			"index":     i,
		}
	}

	query := `
		MATCH (s:BrowserSession {id: $sessionId})
		UNWIND $events AS evt
		CREATE (e:BrowserEvent {
			type: evt.type,
			timestamp: evt.timestamp,
			url: evt.url,
			selector: evt.selector,
			value: evt.value,
			posX: evt.posX,
			posY: evt.posY,
			idx: evt.index
		})
		CREATE (s)-[:HAS_EVENT]->(e)
	`

	return r.db.ExecuteWrite(ctx, query, map[string]any{
		"sessionId": sessionID,
		"events":    eventMaps,
	})
}

// analyzeFlow extracts patterns from session events.
func (r *GraphRecorder) analyzeFlow(ctx context.Context, session *Session) error {
	// Create NEXT_EVENT relationships between sequential events
	linkQuery := `
		MATCH (s:BrowserSession {id: $sessionId})-[:HAS_EVENT]->(e:BrowserEvent)
		WITH e ORDER BY e.idx
		WITH collect(e) AS events
		UNWIND range(0, size(events)-2) AS i
		WITH events[i] AS e1, events[i+1] AS e2
		CREATE (e1)-[:NEXT_EVENT]->(e2)
	`

	if err := r.db.ExecuteWrite(ctx, linkQuery, map[string]any{
		"sessionId": session.ID,
	}); err != nil {
		return err
	}

	// Extract page flow (unique URL transitions)
	pageFlowQuery := `
		MATCH (s:BrowserSession {id: $sessionId})-[:HAS_EVENT]->(e:BrowserEvent)
		WHERE e.type = 'navigate' OR e.url IS NOT NULL
		WITH DISTINCT e.url AS url, min(e.timestamp) AS firstVisit
		ORDER BY firstVisit
		WITH collect({url: url, firstVisit: firstVisit}) AS pages
		UNWIND range(0, size(pages)-2) AS i
		WITH pages[i] AS p1, pages[i+1] AS p2
		MERGE (page1:WebPage {url: p1.url})
		MERGE (page2:WebPage {url: p2.url})
		MERGE (page1)-[r:LEADS_TO]->(page2)
		ON CREATE SET r.count = 1
		ON MATCH SET r.count = r.count + 1
	`

	if err := r.db.ExecuteWrite(ctx, pageFlowQuery, map[string]any{
		"sessionId": session.ID,
	}); err != nil {
		return err
	}

	// Extract interaction patterns (element → action → element)
	patternQuery := `
		MATCH (s:BrowserSession {id: $sessionId})-[:HAS_EVENT]->(e:BrowserEvent)
		WHERE e.selector IS NOT NULL AND e.selector <> ''
		WITH e.selector AS selector, e.type AS action, count(*) AS freq
		WHERE freq > 1
		MERGE (elem:WebElement {selector: selector})
		MERGE (pattern:InteractionPattern {selector: selector, action: action})
		SET pattern.frequency = freq
		MERGE (elem)-[:HAS_PATTERN]->(pattern)
	`

	return r.db.ExecuteWrite(ctx, patternQuery, map[string]any{
		"sessionId": session.ID,
	})
}

// GetSessionFlows retrieves learned page flows.
func (r *GraphRecorder) GetSessionFlows(ctx context.Context, url string) ([]PageFlow, error) {
	query := `
		MATCH (p1:WebPage)-[r:LEADS_TO]->(p2:WebPage)
		WHERE p1.url CONTAINS $urlPart OR p2.url CONTAINS $urlPart
		RETURN p1.url AS from, p2.url AS to, r.count AS count
		ORDER BY r.count DESC
		LIMIT 20
	`

	records, err := r.db.Execute(ctx, query, map[string]any{
		"urlPart": url,
	})
	if err != nil {
		return nil, err
	}

	var flows []PageFlow
	for _, rec := range records {
		flows = append(flows, PageFlow{
			From:  rec["from"].(string),
			To:    rec["to"].(string),
			Count: int(rec["count"].(int64)),
		})
	}

	return flows, nil
}

// PageFlow represents a navigation path between pages.
type PageFlow struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

// GetInteractionPatterns retrieves common interactions for a URL.
func (r *GraphRecorder) GetInteractionPatterns(ctx context.Context, url string) ([]InteractionPattern, error) {
	query := `
		MATCH (s:BrowserSession)-[:HAS_EVENT]->(e:BrowserEvent)
		WHERE e.url CONTAINS $urlPart AND e.selector IS NOT NULL
		WITH e.selector AS selector, e.type AS action, count(*) AS freq
		ORDER BY freq DESC
		LIMIT 50
		RETURN selector, action, freq
	`

	records, err := r.db.Execute(ctx, query, map[string]any{
		"urlPart": url,
	})
	if err != nil {
		return nil, err
	}

	var patterns []InteractionPattern
	for _, rec := range records {
		patterns = append(patterns, InteractionPattern{
			Selector:  rec["selector"].(string),
			Action:    rec["action"].(string),
			Frequency: int(rec["freq"].(int64)),
		})
	}

	return patterns, nil
}

// InteractionPattern represents a learned element interaction.
type InteractionPattern struct {
	Selector  string `json:"selector"`
	Action    string `json:"action"`
	Frequency int    `json:"frequency"`
}

// ReplaySession generates automation actions from a recorded session.
func (r *GraphRecorder) ReplaySession(ctx context.Context, sessionID string) ([]Action, error) {
	query := `
		MATCH (s:BrowserSession {id: $sessionId})-[:HAS_EVENT]->(e:BrowserEvent)
		ORDER BY e.idx
		RETURN e.type AS type, e.selector AS selector, e.value AS value, e.url AS url
	`

	records, err := r.db.Execute(ctx, query, map[string]any{
		"sessionId": sessionID,
	})
	if err != nil {
		return nil, err
	}

	var actions []Action
	for _, rec := range records {
		eventType := rec["type"].(string)

		action := Action{
			Type:  eventType,
			Delay: 200 * time.Millisecond,
		}

		if selector, ok := rec["selector"].(string); ok && selector != "" {
			action.Selector = selector
		}
		if value, ok := rec["value"].(string); ok {
			action.Value = value
		}
		if url, ok := rec["url"].(string); ok && eventType == "navigate" {
			action.Value = url
		}

		actions = append(actions, action)
	}

	return actions, nil
}

// Ensure GraphRecorder implements EventRecorder
var _ EventRecorder = (*GraphRecorder)(nil)
