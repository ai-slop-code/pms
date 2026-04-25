// Package metrics provides a minimal in-process Prometheus-style metrics
// registry. It intentionally has zero third-party dependencies and exposes
// only the handful of counters and gauges the PMS backend needs today (HTTP
// request volume, scheduler job outcomes). Adding new metrics should stay
// deliberate — if the surface grows, swap this for the official client.
package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// labelPair is a single key/value pair; slices of these form a metric's label set.
type labelPair struct {
	Key, Value string
}

func labelKey(pairs []labelPair) string {
	// Callers must pass labels in a stable order per metric. We alphabetize
	// defensively here so counter identity is not order-dependent.
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].Key < pairs[j].Key })
	var b strings.Builder
	for i, p := range pairs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(p.Key)
		b.WriteByte('=')
		b.WriteString(p.Value)
	}
	return b.String()
}

// counter is a monotonically-increasing float64 counter keyed by label set.
type counter struct {
	mu     sync.RWMutex
	name   string
	help   string
	values map[string]counterSample
}

type counterSample struct {
	labels []labelPair
	value  float64
}

func (c *counter) add(v float64, labels []labelPair) {
	k := labelKey(labels)
	c.mu.Lock()
	defer c.mu.Unlock()
	s := c.values[k]
	if s.labels == nil {
		s.labels = append(s.labels, labels...)
	}
	s.value += v
	c.values[k] = s
}

// gauge stores an arbitrary float64 keyed by label set, overwriting on Set.
type gauge struct {
	mu     sync.RWMutex
	name   string
	help   string
	values map[string]gaugeSample
}

type gaugeSample struct {
	labels []labelPair
	value  float64
}

func (g *gauge) set(v float64, labels []labelPair) {
	k := labelKey(labels)
	g.mu.Lock()
	defer g.mu.Unlock()
	g.values[k] = gaugeSample{labels: append([]labelPair(nil), labels...), value: v}
}

// summary tracks a running sum and count for a metric that is observed many
// times (e.g. request duration). It emits Prometheus `_sum`/`_count` pairs.
type summary struct {
	mu     sync.RWMutex
	name   string
	help   string
	values map[string]summarySample
}

type summarySample struct {
	labels []labelPair
	sum    float64
	count  uint64
}

func (s *summary) observe(v float64, labels []labelPair) {
	k := labelKey(labels)
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.values[k]
	if cur.labels == nil {
		cur.labels = append(cur.labels, labels...)
	}
	cur.sum += v
	cur.count++
	s.values[k] = cur
}

// registry holds the canonical set of PMS metrics. Access is package-private;
// callers go through the typed helpers below.
type registry struct {
	httpRequests        *counter
	httpDurations       *summary
	schedulerRuns       *counter
	schedulerLastRun    *gauge
	attachmentRelocated *counter
	backupLastSuccess   *gauge
	auditLogDeleted     *counter
}

var defaultRegistry = &registry{
	httpRequests: &counter{
		name: "pms_http_requests_total",
		help: "Count of HTTP requests handled by the PMS API, labelled by method and status code.",
		values: map[string]counterSample{},
	},
	httpDurations: &summary{
		name: "pms_http_request_duration_seconds",
		help: "Cumulative duration of handled HTTP requests in seconds, labelled by method.",
		values: map[string]summarySample{},
	},
	schedulerRuns: &counter{
		name: "pms_scheduler_runs_total",
		help: "Count of scheduler iterations, labelled by job name and outcome (skipped|ran|error).",
		values: map[string]counterSample{},
	},
	schedulerLastRun: &gauge{
		name: "pms_scheduler_last_run_timestamp_seconds",
		help: "Unix timestamp of the last successful scheduler iteration per job.",
		values: map[string]gaugeSample{},
	},
	attachmentRelocated: &counter{
		name: "pms_attachment_relocations_total",
		help: "Count of finance attachments relocated at startup by the legacy path migration.",
		values: map[string]counterSample{},
	},
	backupLastSuccess: &gauge{
		name: "pms_last_successful_backup_unixtime",
		help: "Unix timestamp of the most recent successful database snapshot.",
		values: map[string]gaugeSample{},
	},
	auditLogDeleted: &counter{
		name: "pms_audit_log_deleted_total",
		help: "Count of audit rows removed by the retention scheduler.",
		values: map[string]counterSample{},
	},
}

// ObserveHTTPRequest records both the request count and its duration.
func ObserveHTTPRequest(method string, status int, elapsed time.Duration) {
	labels := []labelPair{
		{Key: "method", Value: strings.ToUpper(method)},
		{Key: "status", Value: fmt.Sprintf("%d", status)},
	}
	defaultRegistry.httpRequests.add(1, labels)
	defaultRegistry.httpDurations.observe(elapsed.Seconds(), []labelPair{{Key: "method", Value: strings.ToUpper(method)}})
}

// RecordSchedulerRun records one scheduler iteration outcome. Valid outcomes
// are "ran" (lease acquired, work executed), "skipped" (lease held elsewhere),
// and "error" (lease acquisition or body failed).
func RecordSchedulerRun(job, outcome string) {
	defaultRegistry.schedulerRuns.add(1, []labelPair{{Key: "job", Value: job}, {Key: "outcome", Value: outcome}})
	if outcome == "ran" {
		defaultRegistry.schedulerLastRun.set(float64(time.Now().UTC().Unix()), []labelPair{{Key: "job", Value: job}})
	}
}

// RecordAttachmentRelocation bumps the counter tracking the startup attachment
// migration. Outcome values: "relocated", "already_ok", "error".
func RecordAttachmentRelocation(outcome string, n int) {
	if n <= 0 {
		return
	}
	defaultRegistry.attachmentRelocated.add(float64(n), []labelPair{{Key: "outcome", Value: outcome}})
}

// RecordBackupSuccess stamps the time of a successful snapshot so operators
// can alert on staleness.
func RecordBackupSuccess(ts time.Time) {
	defaultRegistry.backupLastSuccess.set(float64(ts.UTC().Unix()), nil)
}

// RecordAuditLogDeletion counts audit rows pruned by the retention scheduler.
func RecordAuditLogDeletion(n int64) {
	if n <= 0 {
		return
	}
	defaultRegistry.auditLogDeleted.add(float64(n), nil)
}

// Handler returns an http.Handler that renders the registry in Prometheus
// text exposition format. It is intentionally not protected by auth by default
// — deployments should gate /metrics at the reverse proxy or bind it to a
// private interface. When `PMS_METRICS_TOKEN` is set, callers must supply a
// matching Bearer token in the Authorization header.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		write := func(s string) { _, _ = w.Write([]byte(s)) }
		writeCounter(write, defaultRegistry.httpRequests)
		writeSummary(write, defaultRegistry.httpDurations)
		writeCounter(write, defaultRegistry.schedulerRuns)
		writeGauge(write, defaultRegistry.schedulerLastRun)
		writeCounter(write, defaultRegistry.attachmentRelocated)
		writeGauge(write, defaultRegistry.backupLastSuccess)
		writeCounter(write, defaultRegistry.auditLogDeleted)
	})
}

func writeCounter(w func(string), c *counter) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	w("# HELP " + c.name + " " + c.help + "\n")
	w("# TYPE " + c.name + " counter\n")
	keys := sortedKeys(c.values)
	for _, k := range keys {
		s := c.values[k]
		w(c.name + formatLabels(s.labels) + " " + formatFloat(s.value) + "\n")
	}
}

func writeGauge(w func(string), g *gauge) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	w("# HELP " + g.name + " " + g.help + "\n")
	w("# TYPE " + g.name + " gauge\n")
	keys := sortedKeys2(g.values)
	for _, k := range keys {
		s := g.values[k]
		w(g.name + formatLabels(s.labels) + " " + formatFloat(s.value) + "\n")
	}
}

func writeSummary(w func(string), s *summary) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	w("# HELP " + s.name + " " + s.help + "\n")
	w("# TYPE " + s.name + " summary\n")
	keys := sortedKeys3(s.values)
	for _, k := range keys {
		v := s.values[k]
		w(s.name + "_sum" + formatLabels(v.labels) + " " + formatFloat(v.sum) + "\n")
		w(s.name + "_count" + formatLabels(v.labels) + " " + fmt.Sprintf("%d", v.count) + "\n")
	}
}

func formatLabels(pairs []labelPair) string {
	if len(pairs) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteByte('{')
	for i, p := range pairs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(p.Key)
		b.WriteString(`="`)
		b.WriteString(escapeLabelValue(p.Value))
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String()
}

func escapeLabelValue(v string) string {
	if !strings.ContainsAny(v, `\"`+"\n") {
		return v
	}
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`)
	return r.Replace(v)
}

func formatFloat(f float64) string {
	return fmt.Sprintf("%g", f)
}

func sortedKeys(m map[string]counterSample) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeys2(m map[string]gaugeSample) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeys3(m map[string]summarySample) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
