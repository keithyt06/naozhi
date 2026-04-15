package patrol

import (
	"net/http"
	"testing"
)

func TestCloudWatchNormalize(t *testing.T) {
	// Simulate an SNS → CloudWatch ALARM notification
	body := []byte(`{
		"Type": "Notification",
		"MessageId": "msg-123",
		"Message": "{\"AlarmName\":\"HighCPU\",\"NewStateValue\":\"ALARM\",\"NewStateReason\":\"CPU > 90%\",\"StateChangeTime\":\"2026-04-15T10:00:00Z\",\"Trigger\":{\"MetricName\":\"CPUUtilization\",\"Namespace\":\"AWS/EC2\"}}",
		"Timestamp": "2026-04-15T10:00:01Z"
	}`)

	n := CloudWatchNormalizer{}
	alert, err := n.Normalize(body, http.Header{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alert.Source != "cloudwatch" {
		t.Errorf("source = %q, want cloudwatch", alert.Source)
	}
	if alert.Title != "HighCPU" {
		t.Errorf("title = %q, want HighCPU", alert.Title)
	}
	if alert.Severity != SeverityCritical {
		t.Errorf("severity = %q, want critical", alert.Severity)
	}
	if alert.Labels["metric"] != "CPUUtilization" {
		t.Errorf("label metric = %q, want CPUUtilization", alert.Labels["metric"])
	}

	// Test OK state → info severity
	bodyOK := []byte(`{
		"Type": "Notification",
		"MessageId": "msg-456",
		"Message": "{\"AlarmName\":\"HighCPU\",\"NewStateValue\":\"OK\",\"NewStateReason\":\"CPU normal\",\"StateChangeTime\":\"2026-04-15T10:05:00Z\",\"Trigger\":{\"MetricName\":\"CPUUtilization\",\"Namespace\":\"AWS/EC2\"}}",
		"Timestamp": "2026-04-15T10:05:01Z"
	}`)
	alertOK, err := n.Normalize(bodyOK, http.Header{})
	if err != nil {
		t.Fatalf("unexpected error for OK state: %v", err)
	}
	if alertOK.Severity != SeverityInfo {
		t.Errorf("OK severity = %q, want info", alertOK.Severity)
	}
}

func TestGrafanaNormalize(t *testing.T) {
	body := []byte(`{
		"ruleName": "High Memory Usage",
		"state": "alerting",
		"message": "Memory usage exceeded 85%",
		"ruleUrl": "https://grafana.example.com/rule/1",
		"evalMatches": [
			{"value": 87.5, "metric": "mem_usage", "tags": {"host": "web-01"}}
		]
	}`)

	n := GrafanaNormalizer{}
	alert, err := n.Normalize(body, http.Header{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alert.Source != "grafana" {
		t.Errorf("source = %q, want grafana", alert.Source)
	}
	if alert.Title != "High Memory Usage" {
		t.Errorf("title = %q, want High Memory Usage", alert.Title)
	}
	if alert.Severity != SeverityCritical {
		t.Errorf("severity = %q, want critical (alerting state)", alert.Severity)
	}
	if alert.Labels["rule_url"] != "https://grafana.example.com/rule/1" {
		t.Errorf("missing rule_url label")
	}

	// Test ok state
	bodyOK := []byte(`{"ruleName": "High Memory Usage", "state": "ok", "message": "resolved"}`)
	alertOK, err := n.Normalize(bodyOK, http.Header{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alertOK.Severity != SeverityInfo {
		t.Errorf("ok severity = %q, want info", alertOK.Severity)
	}
}

func TestDatadogNormalize(t *testing.T) {
	body := []byte(`{
		"title": "Disk space critical on web-02",
		"alert_type": "error",
		"body": "Disk usage at 95% on /dev/sda1",
		"tags": ["env:production", "service:api"],
		"event_type": "alert"
	}`)

	n := DatadogNormalizer{}
	alert, err := n.Normalize(body, http.Header{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alert.Source != "datadog" {
		t.Errorf("source = %q, want datadog", alert.Source)
	}
	if alert.Severity != SeverityCritical {
		t.Errorf("severity = %q, want critical (error type)", alert.Severity)
	}
	if alert.Labels["tag_env"] != "production" {
		t.Errorf("tag_env = %q, want production", alert.Labels["tag_env"])
	}
	if alert.Labels["tag_service"] != "api" {
		t.Errorf("tag_service = %q, want api", alert.Labels["tag_service"])
	}

	// warning type
	bodyWarn := []byte(`{"title": "Slow queries", "alert_type": "warning", "body": "p99 > 500ms", "tags": []}`)
	alertWarn, err := n.Normalize(bodyWarn, http.Header{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alertWarn.Severity != SeverityWarning {
		t.Errorf("warning severity = %q, want warning", alertWarn.Severity)
	}
}

func TestAutoDetectSource(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		headers http.Header
		want    string
	}{
		{
			name:    "explicit header",
			body:    []byte(`{}`),
			headers: http.Header{"X-Alert-Source": {"grafana"}},
			want:    "grafana",
		},
		{
			name:    "SNS headers",
			body:    []byte(`{}`),
			headers: http.Header{"X-Amz-Sns-Topic-Arn": {"arn:aws:sns:us-east-1:123:alerts"}},
			want:    "cloudwatch",
		},
		{
			name:    "body source field",
			body:    []byte(`{"source": "Datadog"}`),
			headers: http.Header{},
			want:    "datadog",
		},
		{
			name:    "grafana body detection",
			body:    []byte(`{"ruleName": "test-rule", "state": "alerting"}`),
			headers: http.Header{},
			want:    "grafana",
		},
		{
			name:    "datadog body detection",
			body:    []byte(`{"title": "test", "alert_type": "error"}`),
			headers: http.Header{},
			want:    "datadog",
		},
		{
			name:    "unknown",
			body:    []byte(`{"foo": "bar"}`),
			headers: http.Header{},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectAlertSource(tt.body, tt.headers)
			if got != tt.want {
				t.Errorf("DetectAlertSource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCustomNormalize(t *testing.T) {
	body := []byte(`{
		"alert_name": "custom check",
		"severity": "critical",
		"summary": "something is wrong",
		"labels": {"service": "backend"}
	}`)

	n := CustomNormalizer{}
	alert, err := n.Normalize(body, http.Header{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if alert.Title != "custom check" {
		t.Errorf("title = %q, want 'custom check'", alert.Title)
	}
	if alert.Severity != SeverityCritical {
		t.Errorf("severity = %q, want critical", alert.Severity)
	}
}

func TestGetNormalizerUnknown(t *testing.T) {
	_, err := GetNormalizer("prometheus")
	if err == nil {
		t.Fatal("expected error for unknown source")
	}
}

func TestCloudWatchNormalizeBadJSON(t *testing.T) {
	n := CloudWatchNormalizer{}
	_, err := n.Normalize([]byte(`not json`), http.Header{})
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

// I5: Bad JSON tests for Grafana and Datadog normalizers.
func TestGrafanaNormalizeBadJSON(t *testing.T) {
	n := GrafanaNormalizer{}
	_, err := n.Normalize([]byte(`not json`), http.Header{})
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}

func TestDatadogNormalizeBadJSON(t *testing.T) {
	n := DatadogNormalizer{}
	_, err := n.Normalize([]byte(`not json`), http.Header{})
	if err == nil {
		t.Fatal("expected error for bad JSON")
	}
}
