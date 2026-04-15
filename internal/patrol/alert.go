package patrol

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Severity classifies alert urgency.
type Severity string

const (
	SeverityCritical Severity = "critical"
	SeverityWarning  Severity = "warning"
	SeverityInfo     Severity = "info"
)

// Alert is the normalised representation of an external monitoring alert.
type Alert struct {
	ID          string            `json:"id"`
	Source      string            `json:"source"` // cloudwatch, grafana, datadog, custom
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Severity    Severity          `json:"severity"`
	Timestamp   time.Time         `json:"timestamp"`
	Labels      map[string]string `json:"labels,omitempty"`
	RawPayload  json.RawMessage   `json:"raw_payload,omitempty"`
}

// AlertNormalizer converts a raw webhook body into a normalised Alert.
type AlertNormalizer interface {
	Normalize(body []byte, headers http.Header) (*Alert, error)
}

// --- CloudWatch Normalizer ---

// CloudWatchNormalizer handles CloudWatch Alarms delivered via SNS HTTPS.
type CloudWatchNormalizer struct{}

// cloudWatchSNS is the subset of an SNS notification we parse.
type cloudWatchSNS struct {
	Type      string `json:"Type"`
	MessageID string `json:"MessageId"`
	Message   string `json:"Message"` // JSON-encoded AlarmState
	Timestamp string `json:"Timestamp"`
}

type cloudWatchAlarm struct {
	AlarmName        string `json:"AlarmName"`
	NewStateValue    string `json:"NewStateValue"`    // ALARM, OK, INSUFFICIENT_DATA
	NewStateReason   string `json:"NewStateReason"`
	StateChangeTime  string `json:"StateChangeTime"`
	AlarmDescription string `json:"AlarmDescription"`
	Trigger          struct {
		MetricName string `json:"MetricName"`
		Namespace  string `json:"Namespace"`
	} `json:"Trigger"`
}

func (CloudWatchNormalizer) Normalize(body []byte, headers http.Header) (*Alert, error) {
	var sns cloudWatchSNS
	if err := json.Unmarshal(body, &sns); err != nil {
		return nil, fmt.Errorf("parse SNS envelope: %w", err)
	}

	// SNS SubscriptionConfirmation — pass through without error
	if sns.Type == "SubscriptionConfirmation" {
		return &Alert{
			ID:          generateID(),
			Source:      "cloudwatch",
			Title:       "SNS Subscription Confirmation",
			Description: "SubscriptionConfirmation received; manual confirm required",
			Severity:    SeverityInfo,
			Timestamp:   time.Now(),
			RawPayload:  body,
		}, nil
	}

	var alarm cloudWatchAlarm
	if err := json.Unmarshal([]byte(sns.Message), &alarm); err != nil {
		return nil, fmt.Errorf("parse CloudWatch alarm message: %w", err)
	}

	sev := mapCloudWatchSeverity(alarm.NewStateValue)
	ts, _ := time.Parse(time.RFC3339, alarm.StateChangeTime)
	if ts.IsZero() {
		ts = time.Now()
	}

	labels := map[string]string{
		"alarm_name":   alarm.AlarmName,
		"state":        alarm.NewStateValue,
		"metric":       alarm.Trigger.MetricName,
		"namespace":    alarm.Trigger.Namespace,
	}

	return &Alert{
		ID:          generateID(),
		Source:      "cloudwatch",
		Title:       alarm.AlarmName,
		Description: alarm.NewStateReason,
		Severity:    sev,
		Timestamp:   ts,
		Labels:      labels,
		RawPayload:  body,
	}, nil
}

func mapCloudWatchSeverity(state string) Severity {
	switch strings.ToUpper(state) {
	case "ALARM":
		return SeverityCritical
	case "INSUFFICIENT_DATA":
		return SeverityWarning
	default: // OK
		return SeverityInfo
	}
}

// --- Grafana Normalizer ---

// GrafanaNormalizer handles Grafana webhook notifications.
type GrafanaNormalizer struct{}

type grafanaWebhook struct {
	RuleName    string `json:"ruleName"`
	State       string `json:"state"` // alerting, ok, no_data, pending
	Message     string `json:"message"`
	RuleURL     string `json:"ruleUrl"`
	EvalMatches []struct {
		Value  float64           `json:"value"`
		Metric string            `json:"metric"`
		Tags   map[string]string `json:"tags"`
	} `json:"evalMatches"`
}

func (GrafanaNormalizer) Normalize(body []byte, headers http.Header) (*Alert, error) {
	var gw grafanaWebhook
	if err := json.Unmarshal(body, &gw); err != nil {
		return nil, fmt.Errorf("parse Grafana webhook: %w", err)
	}
	if gw.RuleName == "" {
		return nil, fmt.Errorf("missing ruleName in Grafana payload")
	}

	sev := mapGrafanaSeverity(gw.State)
	labels := map[string]string{
		"rule_name": gw.RuleName,
		"state":     gw.State,
	}
	if gw.RuleURL != "" {
		labels["rule_url"] = gw.RuleURL
	}
	for i, em := range gw.EvalMatches {
		labels[fmt.Sprintf("metric_%d", i)] = em.Metric
	}

	return &Alert{
		ID:          generateID(),
		Source:      "grafana",
		Title:       gw.RuleName,
		Description: gw.Message,
		Severity:    sev,
		Timestamp:   time.Now(),
		Labels:      labels,
		RawPayload:  body,
	}, nil
}

func mapGrafanaSeverity(state string) Severity {
	switch strings.ToLower(state) {
	case "alerting":
		return SeverityCritical
	case "no_data":
		return SeverityWarning
	default: // ok, pending
		return SeverityInfo
	}
}

// --- Datadog Normalizer ---

// DatadogNormalizer handles Datadog webhook notifications.
type DatadogNormalizer struct{}

type datadogWebhook struct {
	Title     string   `json:"title"`
	AlertType string   `json:"alert_type"` // error, warning, info, success
	Body      string   `json:"body"`
	Tags      []string `json:"tags"`
	EventType string   `json:"event_type"`
}

func (DatadogNormalizer) Normalize(body []byte, headers http.Header) (*Alert, error) {
	var dd datadogWebhook
	if err := json.Unmarshal(body, &dd); err != nil {
		return nil, fmt.Errorf("parse Datadog webhook: %w", err)
	}
	if dd.Title == "" {
		return nil, fmt.Errorf("missing title in Datadog payload")
	}

	sev := mapDatadogSeverity(dd.AlertType)
	labels := map[string]string{
		"alert_type": dd.AlertType,
		"event_type": dd.EventType,
	}
	for _, tag := range dd.Tags {
		parts := strings.SplitN(tag, ":", 2)
		if len(parts) == 2 {
			labels["tag_"+parts[0]] = parts[1]
		}
	}

	return &Alert{
		ID:          generateID(),
		Source:      "datadog",
		Title:       dd.Title,
		Description: dd.Body,
		Severity:    sev,
		Timestamp:   time.Now(),
		Labels:      labels,
		RawPayload:  body,
	}, nil
}

func mapDatadogSeverity(alertType string) Severity {
	switch strings.ToLower(alertType) {
	case "error":
		return SeverityCritical
	case "warning":
		return SeverityWarning
	default: // info, success
		return SeverityInfo
	}
}

// --- Auto-detect + Dispatch ---

// supportedSources lists all recognised alert source names for error messages.
var supportedSources = []string{"cloudwatch", "grafana", "datadog", "custom"}

// DetectAlertSource inspects headers and body to determine the alert source.
// Priority: explicit X-Alert-Source header > SNS headers > body "source" field.
func DetectAlertSource(body []byte, headers http.Header) string {
	// Explicit header takes highest priority
	if src := headers.Get("X-Alert-Source"); src != "" {
		return strings.ToLower(src)
	}

	// SNS topic header indicates CloudWatch (via SNS)
	if headers.Get("X-Amz-Sns-Topic-Arn") != "" || headers.Get("X-Amz-Sns-Message-Type") != "" {
		return "cloudwatch"
	}

	// Try to detect from body
	var probe struct {
		Source string `json:"source"`
		// Grafana-specific
		RuleName string `json:"ruleName"`
		// Datadog-specific
		AlertType string `json:"alert_type"`
	}
	if json.Unmarshal(body, &probe) == nil {
		if probe.Source != "" {
			return strings.ToLower(probe.Source)
		}
		if probe.RuleName != "" {
			return "grafana"
		}
		if probe.AlertType != "" {
			return "datadog"
		}
	}

	return ""
}

// GetNormalizer returns the appropriate normalizer for a source string.
func GetNormalizer(source string) (AlertNormalizer, error) {
	switch strings.ToLower(source) {
	case "cloudwatch":
		return CloudWatchNormalizer{}, nil
	case "grafana":
		return GrafanaNormalizer{}, nil
	case "datadog":
		return DatadogNormalizer{}, nil
	case "custom":
		return CustomNormalizer{}, nil
	default:
		return nil, fmt.Errorf("unknown alert source %q, supported: %s", source, strings.Join(supportedSources, ", "))
	}
}

// --- Custom Normalizer ---

// CustomNormalizer accepts arbitrary JSON with conventional fields.
type CustomNormalizer struct{}

func (CustomNormalizer) Normalize(body []byte, headers http.Header) (*Alert, error) {
	var raw struct {
		AlertName string            `json:"alert_name"`
		Title     string            `json:"title"`
		Severity  string            `json:"severity"`
		Summary   string            `json:"summary"`
		Labels    map[string]string `json:"labels"`
	}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("parse custom alert: %w", err)
	}

	title := raw.AlertName
	if title == "" {
		title = raw.Title
	}
	if title == "" {
		title = "custom alert"
	}

	sev := SeverityWarning
	switch strings.ToLower(raw.Severity) {
	case "critical":
		sev = SeverityCritical
	case "info":
		sev = SeverityInfo
	}

	return &Alert{
		ID:          generateID(),
		Source:      "custom",
		Title:       title,
		Description: raw.Summary,
		Severity:    sev,
		Timestamp:   time.Now(),
		Labels:      raw.Labels,
		RawPayload:  body,
	}, nil
}
