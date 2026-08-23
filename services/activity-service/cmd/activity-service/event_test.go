package main

import (
	"encoding/json"
	"github.com/segmentio/kafka-go"
	"testing"
)

func TestEventPayloadDecodesVersionedAndLegacyEnvelopes(t *testing.T) {
	for name, raw := range map[string]string{
		"versioned": `{"schema_version":1,"event_id":"e1","event_type":"task.created","task":{"id":"t1","owner_id":"u1"},"occurred_at":"2026-01-01T00:00:00Z"}`,
		"legacy":    `{"EventID":"e2","EventType":"task.updated","Task":{"ID":"t2","OwnerID":"u2"},"OccurredAt":"2026-01-01T00:00:00Z"}`,
	} {
		t.Run(name, func(t *testing.T) {
			var event eventPayload
			if err := json.Unmarshal([]byte(raw), &event); err != nil {
				t.Fatal(err)
			}
			if event.EventID == "" || event.EventType == "" || event.Task.ID == "" || event.Task.OwnerID == "" {
				t.Fatalf("incomplete event: %#v", event)
			}
		})
	}
}

func TestActivityReaderWatchesTopicsCreatedAfterStartup(t *testing.T) {
	config := activityReaderConfig("kafka:9092")
	if !config.WatchPartitionChanges || config.PartitionWatchInterval <= 0 || config.StartOffset != kafka.FirstOffset {
		t.Fatalf("unexpected reader recovery settings: watch=%v interval=%s offset=%d", config.WatchPartitionChanges, config.PartitionWatchInterval, config.StartOffset)
	}
}
