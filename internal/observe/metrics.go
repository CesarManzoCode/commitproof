package observe

import (
	"context"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/CesarManzoCode/commitproof/internal/params"
)

// Metric scrapes (CONTRACT P23, §5.4; ADR-0038). Every multigateway,
// multipooler and multiorch serves the OpenTelemetry Prometheus exporter's
// /metrics on params.MetricsPort; no HTTP port of the pinned commit serves
// /metrics. Each scrape is reduced to the integer families of ADR-0038's
// table — the ones the report draws — and recorded as one observation; the
// exposition text is not stored (P25).

// MetricsBodyLimit bounds one scrape's body (a full exposition is 70–120 KB).
const MetricsBodyLimit = 4 << 20

// Component kinds that serve metrics.
const (
	KindGateway      = "gateway"
	KindPooler       = "pooler"
	KindOrchestrator = "orchestrator"
)

// Family is one recorded metric family (ADR-0038).
type Family struct {
	Key     string  // the observation key
	Series  string  // the Prometheus series name
	Label   string  // a label filter's name ("" for none)
	Value   string  // the filter's value
	Not     bool    // the filter requires Label ≠ Value instead of =
	Scale   float64 // 1, or 1000 for seconds → ms
	Counter bool    // absent ⇒ 0; otherwise absent ⇒ not recorded
	Kinds   []string
}

var all = []string{KindGateway, KindPooler, KindOrchestrator}

// Families is ADR-0038's table, in its order.
var Families = []Family{
	{Key: "gw_client_connections", Series: "mg_gateway_client_connections", Scale: 1, Kinds: []string{KindGateway}},
	{Key: "gw_queries", Series: "mg_gateway_query_duration_seconds_count", Scale: 1, Counter: true, Kinds: []string{KindGateway}},
	{Key: "gw_query_errors", Series: "mg_gateway_query_duration_seconds_count", Label: "status", Value: "ok", Not: true, Scale: 1, Counter: true, Kinds: []string{KindGateway}},
	{Key: "gw_buffered", Series: "multigateway_buffer_requests_buffered_total", Scale: 1, Counter: true, Kinds: []string{KindGateway}},
	{Key: "pl_up", Series: "mg_pooler_up", Scale: 1, Kinds: []string{KindPooler}},
	{Key: "pl_serving_transitions", Series: "mg_pooler_serving_transitions_total", Scale: 1, Counter: true, Kinds: []string{KindPooler}},
	{Key: "pl_server_conns_active", Series: "mg_pooler_server_connections", Label: "state", Value: "active", Scale: 1, Kinds: []string{KindPooler}},
	{Key: "pl_client_waiting", Series: "mg_pooler_client_waiting_connections", Scale: 1, Kinds: []string{KindPooler}},
	{Key: "pl_conn_open_errors", Series: "mg_pooler_server_conn_open_errors_total", Scale: 1, Counter: true, Kinds: []string{KindPooler}},
	{Key: "pl_queries", Series: "mg_pooler_query_duration_seconds_count", Scale: 1, Counter: true, Kinds: []string{KindPooler}},
	{Key: "pl_replication_lag_ms", Series: "mg_pooler_replication_lag_seconds", Scale: 1000, Kinds: []string{KindPooler}},
	{Key: "or_recovery_actions", Series: "multiorch_recovery_action_duration_milliseconds_count", Scale: 1, Counter: true, Kinds: []string{KindOrchestrator}},
	{Key: "or_recovery_errors", Series: "multiorch_recovery_errors_total", Scale: 1, Counter: true, Kinds: []string{KindOrchestrator}},
	{Key: "or_detected_problems", Series: "multiorch_recovery_detected_problems", Scale: 1, Kinds: []string{KindOrchestrator}},
	{Key: "or_streams_connected", Series: "multiorch_recovery_stream_connected", Scale: 1, Kinds: []string{KindOrchestrator}},
	{Key: "cpu_ms", Series: "process_cpu_time_seconds_total", Scale: 1000, Counter: true, Kinds: all},
}

// MetricsTarget is one component that serves /metrics.
type MetricsTarget struct {
	Name, Kind, Addr string // Addr is host:port
}

// MetricsTargets returns the scrape targets of the topology (CONTRACT
// §5.1) on the cpnet prefix: gateways .31–.33, poolers .11–.14 (the pg-nX
// address: the pooler shares that network namespace), orchestrators
// .21–.23. Addresses, not names: no DNS lookup (LIM-35).
func MetricsTargets(prefix string, poolers []string) []MetricsTarget {
	port := strconv.Itoa(params.MetricsPort)
	var ts []MetricsTarget
	for i := 1; i <= 3; i++ {
		ts = append(ts, MetricsTarget{fmt.Sprintf("gw-z%d", i), KindGateway, net.JoinHostPort(fmt.Sprintf("%s.%d", prefix, 30+i), port)})
	}
	for _, n := range poolers {
		ts = append(ts, PoolerMetricsTarget(prefix, n))
	}
	for i := 1; i <= 3; i++ {
		ts = append(ts, MetricsTarget{fmt.Sprintf("orch-z%d", i), KindOrchestrator, net.JoinHostPort(fmt.Sprintf("%s.%d", prefix, 20+i), port)})
	}
	return ts
}

// PoolerMetricsTarget is node nX's pooler (pg-nX's address, .1X).
func PoolerMetricsTarget(prefix, node string) MetricsTarget {
	n, _ := strconv.Atoi(strings.TrimPrefix(node, "n"))
	return MetricsTarget{"pooler-" + node, KindPooler, net.JoinHostPort(fmt.Sprintf("%s.%d", prefix, 10+n), strconv.Itoa(params.MetricsPort))}
}

// Scraper fetches /metrics with P23's interval as the request timeout.
type Scraper struct {
	client *http.Client
}

// NewScraper returns a scraper with its own transport (one idle
// connection per target).
func NewScraper() *Scraper {
	tr := &http.Transport{
		DialContext:           (&net.Dialer{Timeout: params.MetricsScrapeInterval}).DialContext,
		MaxIdleConnsPerHost:   1,
		ResponseHeaderTimeout: params.MetricsScrapeInterval,
	}
	return &Scraper{client: &http.Client{Timeout: params.MetricsScrapeInterval, Transport: tr}}
}

// Close releases idle connections.
func (s *Scraper) Close() { s.client.CloseIdleConnections() }

// Scrape fetches t's /metrics once and returns the observation payload.
func (s *Scraper) Scrape(ctx context.Context, t MetricsTarget) map[string]any {
	start := time.Now()
	obs := map[string]any{"src": "metrics", "target": t.Name}
	fail := func(err error) map[string]any {
		obs["ok"], obs["latency_ms"] = false, time.Since(start).Milliseconds()
		obs["error"] = truncate(err.Error(), 256)
		return obs
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+t.Addr+"/metrics", nil)
	if err != nil {
		return fail(err)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return fail(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, MetricsBodyLimit+1))
	if err != nil {
		return fail(err)
	}
	switch {
	case resp.StatusCode != http.StatusOK:
		return fail(fmt.Errorf("HTTP %d", resp.StatusCode))
	case len(body) > MetricsBodyLimit:
		return fail(fmt.Errorf("body exceeds %d bytes", MetricsBodyLimit))
	}
	v, err := Reduce(string(body), t.Kind)
	if err != nil {
		return fail(err)
	}
	obs["ok"], obs["latency_ms"], obs["bytes"], obs["v"] = true, time.Since(start).Milliseconds(), int64(len(body)), v
	return obs
}

// Reduce parses a Prometheus text exposition and returns kind's families
// of ADR-0038 as integers: per family, the sum of the finite values of the
// series whose name and label filter match, times the scale, rounded half
// away from zero. A counter family without a matching series is 0; a gauge
// family without one is omitted. An exposition without any sample line is
// an error (an HTML page is not an exposition).
func Reduce(text, kind string) (map[string]any, error) {
	sums := map[string]float64{}
	seen := map[string]bool{}
	samples := 0
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' {
			continue
		}
		name, labels, val, err := parseSample(line)
		if err != nil {
			return nil, err
		}
		samples++
		for _, f := range Families {
			if f.Series != name || !hasKind(f, kind) {
				continue
			}
			if f.Label != "" && (labels[f.Label] == f.Value) == f.Not {
				continue
			}
			if math.IsNaN(val) || math.IsInf(val, 0) {
				continue
			}
			sums[f.Key] += val
			seen[f.Key] = true
		}
	}
	if samples == 0 {
		return nil, fmt.Errorf("no sample line in the exposition")
	}
	out := map[string]any{}
	for _, f := range Families {
		if !hasKind(f, kind) {
			continue
		}
		switch {
		case seen[f.Key]:
			out[f.Key] = int64(math.Round(sums[f.Key] * f.Scale))
		case f.Counter:
			out[f.Key] = int64(0)
		}
	}
	return out, nil
}

func hasKind(f Family, kind string) bool {
	for _, k := range f.Kinds {
		if k == kind {
			return true
		}
	}
	return false
}

// parseSample parses `name{l="v",…} value [timestamp]` (the Prometheus text
// format; label values may contain escaped quotes, backslashes, newlines).
func parseSample(line string) (string, map[string]string, float64, error) {
	i := strings.IndexAny(line, "{ \t")
	if i <= 0 {
		return "", nil, 0, fmt.Errorf("malformed sample line %q", truncate(line, 80))
	}
	name, rest := line[:i], line[i:]
	labels := map[string]string{}
	if rest[0] == '{' {
		j := 1
		for {
			for j < len(rest) && (rest[j] == ' ' || rest[j] == ',') {
				j++
			}
			if j >= len(rest) {
				return "", nil, 0, fmt.Errorf("unterminated labels in %q", truncate(line, 80))
			}
			if rest[j] == '}' {
				j++
				break
			}
			eq := strings.IndexByte(rest[j:], '=')
			if eq < 0 || j+eq+1 >= len(rest) || rest[j+eq+1] != '"' {
				return "", nil, 0, fmt.Errorf("malformed label in %q", truncate(line, 80))
			}
			key := strings.TrimSpace(rest[j : j+eq])
			k := j + eq + 2
			var b strings.Builder
			for ; k < len(rest) && rest[k] != '"'; k++ {
				if rest[k] == '\\' && k+1 < len(rest) {
					k++
					switch rest[k] {
					case 'n':
						b.WriteByte('\n')
					default:
						b.WriteByte(rest[k])
					}
					continue
				}
				b.WriteByte(rest[k])
			}
			if k >= len(rest) {
				return "", nil, 0, fmt.Errorf("unterminated label value in %q", truncate(line, 80))
			}
			labels[key] = b.String()
			j = k + 1
		}
		rest = rest[j:]
	}
	fields := strings.Fields(rest)
	if len(fields) < 1 || len(fields) > 2 {
		return "", nil, 0, fmt.Errorf("malformed sample value in %q", truncate(line, 80))
	}
	v, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "", nil, 0, fmt.Errorf("sample value in %q: %w", truncate(line, 80), err)
	}
	return name, labels, v, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
