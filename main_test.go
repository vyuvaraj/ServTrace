package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"servtrace/pkg/server"
	"servtrace/pkg/store"
)

func TestServTraceCollector(t *testing.T) {
	ts := store.NewStore(2) // limit to 2 traces for testing eviction
	srv := server.NewServer(ts)
	testServer := httptest.NewServer(srv.Handler())
	defer testServer.Close()

	traceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	span1ID := "00f067aa0ba902b7"
	span2ID := "3e63f565c553856a"

	// Mock OTLP payload matching exportSpans in ServShared
	nowNano := time.Now().UnixNano()
	end1Nano := nowNano + int64(100*time.Millisecond)
	start2Nano := nowNano + int64(10*time.Millisecond)
	end2Nano := nowNano + int64(80*time.Millisecond)

	payload := map[string]interface{}{
		"resourceSpans": []interface{}{
			map[string]interface{}{
				"resource": map[string]interface{}{
					"attributes": []interface{}{
						map[string]interface{}{"key": "service.name", "value": map[string]interface{}{"stringValue": "test-service"}},
					},
				},
				"scopeSpans": []interface{}{
					map[string]interface{}{
						"scope": map[string]interface{}{"name": "servverse-shared"},
						"spans": []interface{}{
							map[string]interface{}{
								"traceId":           traceID,
								"spanId":            span1ID,
								"name":              "HTTP GET /users",
								"kind":              2, // Server
								"startTimeUnixNano": fmt.Sprintf("%d", nowNano),
								"endTimeUnixNano":   fmt.Sprintf("%d", end1Nano),
								"status":            map[string]interface{}{"code": 1}, // OK
							},
							map[string]interface{}{
								"traceId":           traceID,
								"spanId":            span2ID,
								"parentSpanId":      span1ID,
								"name":              "Database SELECT users",
								"kind":              3, // Client
								"startTimeUnixNano": fmt.Sprintf("%d", start2Nano),
								"endTimeUnixNano":   fmt.Sprintf("%d", end2Nano),
								"status":            map[string]interface{}{"code": 2}, // Error
								"attributes": []interface{}{
									map[string]interface{}{"key": "db.statement", "value": map[string]interface{}{"stringValue": "SELECT * FROM users"}},
								},
							},
						},
					},
				},
			},
		},
	}

	// 1. Ingest Traces
	body, _ := json.Marshal(payload)
	resp, err := http.Post(testServer.URL+"/v1/traces", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("failed to make ingestion request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", resp.StatusCode)
	}

	// 2. Query Traces List
	listResp, err := http.Get(testServer.URL + "/api/traces")
	if err != nil {
		t.Fatalf("failed to query traces list: %v", err)
	}
	defer listResp.Body.Close()

	var traces []store.TraceSummary
	if err := json.NewDecoder(listResp.Body).Decode(&traces); err != nil {
		t.Fatalf("failed to decode list: %v", err)
	}

	if len(traces) != 1 {
		t.Fatalf("expected 1 trace, got %d", len(traces))
	}

	summary := traces[0]
	if summary.TraceID != traceID {
		t.Errorf("expected traceId %s, got %s", traceID, summary.TraceID)
	}
	if summary.RootName != "HTTP GET /users" {
		t.Errorf("expected rootName 'HTTP GET /users', got %s", summary.RootName)
	}
	if summary.Service != "test-service" {
		t.Errorf("expected service 'test-service', got %s", summary.Service)
	}
	if summary.TotalSpans != 2 {
		t.Errorf("expected 2 spans, got %d", summary.TotalSpans)
	}
	if summary.ErrorCount != 1 {
		t.Errorf("expected 1 error, got %d", summary.ErrorCount)
	}

	// 3. Query Trace Tree Waterfall
	treeResp, err := http.Get(testServer.URL + "/api/traces/" + traceID)
	if err != nil {
		t.Fatalf("failed to query tree: %v", err)
	}
	defer treeResp.Body.Close()

	var root store.SpanNode
	if err := json.NewDecoder(treeResp.Body).Decode(&root); err != nil {
		t.Fatalf("failed to decode tree root: %v", err)
	}

	if root.Span.SpanID != span1ID {
		t.Errorf("expected root spanID %s, got %s", span1ID, root.Span.SpanID)
	}
	if len(root.Children) != 1 {
		t.Fatalf("expected root to have 1 child, got %d", len(root.Children))
	}

	child := root.Children[0]
	if child.Span.SpanID != span2ID {
		t.Errorf("expected child spanID %s, got %s", span2ID, child.Span.SpanID)
	}
	if child.Span.ParentSpanID != span1ID {
		t.Errorf("expected child parentID %s, got %s", span1ID, child.Span.ParentSpanID)
	}

	// Validate DB statement attribute
	dbStatement, exists := child.Span.Attributes["db.statement"]
	if !exists || dbStatement != "SELECT * FROM users" {
		t.Errorf("expected db.statement attribute 'SELECT * FROM users', got %v", dbStatement)
	}

	// 4. Test Eviction
	// Ingest Trace 2
	payload2 := map[string]interface{}{
		"resourceSpans": []interface{}{
			map[string]interface{}{
				"resource": map[string]interface{}{
					"attributes": []interface{}{
						map[string]interface{}{"key": "service.name", "value": map[string]interface{}{"stringValue": "test-service"}},
					},
				},
				"scopeSpans": []interface{}{
					map[string]interface{}{
						"spans": []interface{}{
							map[string]interface{}{
								"traceId":           "trace2",
								"spanId":            "spanX",
								"name":              "Span 2",
								"startTimeUnixNano": fmt.Sprintf("%d", nowNano),
								"endTimeUnixNano":   fmt.Sprintf("%d", end1Nano),
							},
						},
					},
				},
			},
		},
	}
	body2, _ := json.Marshal(payload2)
	_, _ = http.Post(testServer.URL+"/v1/traces", "application/json", bytes.NewReader(body2))

	// Ingest Trace 3
	payload3 := map[string]interface{}{
		"resourceSpans": []interface{}{
			map[string]interface{}{
				"resource": map[string]interface{}{
					"attributes": []interface{}{
						map[string]interface{}{"key": "service.name", "value": map[string]interface{}{"stringValue": "test-service"}},
					},
				},
				"scopeSpans": []interface{}{
					map[string]interface{}{
						"spans": []interface{}{
							map[string]interface{}{
								"traceId":           "trace3",
								"spanId":            "spanY",
								"name":              "Span 3",
								"startTimeUnixNano": fmt.Sprintf("%d", nowNano),
								"endTimeUnixNano":   fmt.Sprintf("%d", end1Nano),
							},
						},
					},
				},
			},
		},
	}
	body3, _ := json.Marshal(payload3)
	_, _ = http.Post(testServer.URL+"/v1/traces", "application/json", bytes.NewReader(body3))

	// List should now only have Trace 2 and Trace 3, while Trace 1 is evicted
	listResp2, _ := http.Get(testServer.URL + "/api/traces")
	var traces2 []store.TraceSummary
	_ = json.NewDecoder(listResp2.Body).Decode(&traces2)
	listResp2.Body.Close()

	if len(traces2) != 2 {
		t.Fatalf("expected 2 traces, got %d", len(traces2))
	}

	for _, tSum := range traces2 {
		if tSum.TraceID == traceID {
			t.Errorf("expected Trace 1 (%s) to be evicted, but it is still in memory", traceID)
		}
	}
}
