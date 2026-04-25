package metrics

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandler_EmitsCountersAndSummary(t *testing.T) {
	ObserveHTTPRequest("GET", 200, 15*time.Millisecond)
	ObserveHTTPRequest("GET", 200, 5*time.Millisecond)
	ObserveHTTPRequest("POST", 500, 30*time.Millisecond)
	RecordSchedulerRun("occupancy_sync", "ran")
	RecordSchedulerRun("occupancy_sync", "skipped")
	RecordSchedulerRun("nuki_cleanup", "error")
	RecordAttachmentRelocation("relocated", 3)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	Handler().ServeHTTP(rec, req)
	body := rec.Body.String()

	for _, want := range []string{
		`pms_http_requests_total{method="GET",status="200"} 2`,
		`pms_http_requests_total{method="POST",status="500"} 1`,
		`pms_http_request_duration_seconds_count{method="GET"} 2`,
		`pms_scheduler_runs_total{job="occupancy_sync",outcome="ran"} 1`,
		`pms_scheduler_runs_total{job="nuki_cleanup",outcome="error"} 1`,
		`pms_scheduler_last_run_timestamp_seconds{job="occupancy_sync"}`,
		`pms_attachment_relocations_total{outcome="relocated"} 3`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing metric line %q\nbody:\n%s", want, body)
		}
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/plain") {
		t.Errorf("content-type=%q", ct)
	}
}
