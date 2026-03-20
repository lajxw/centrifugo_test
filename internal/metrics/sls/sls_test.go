package sls

import (
	"testing"

	"github.com/FZambia/eagle"
	"github.com/stretchr/testify/assert"
)

func TestBuildMetricName(t *testing.T) {
	tests := []struct {
		name        string
		item        eagle.Metric
		metricValue eagle.MetricValue
		expected    string
	}{
		{
			name:        "full name with namespace subsystem name and value name",
			item:        eagle.Metric{Namespace: "centrifugo", Subsystem: "node", Name: "clients"},
			metricValue: eagle.MetricValue{Name: "total"},
			expected:    "centrifugo_node_clients_total",
		},
		{
			name:        "no value name",
			item:        eagle.Metric{Namespace: "centrifugo", Subsystem: "node", Name: "clients"},
			metricValue: eagle.MetricValue{Name: ""},
			expected:    "centrifugo_node_clients",
		},
		{
			name:        "namespace and subsystem only",
			item:        eagle.Metric{Namespace: "centrifugo", Subsystem: "proxy"},
			metricValue: eagle.MetricValue{Name: ""},
			expected:    "centrifugo_proxy",
		},
		{
			name:        "empty metric",
			item:        eagle.Metric{},
			metricValue: eagle.MetricValue{},
			expected:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildMetricName(tt.item, tt.metricValue)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildLabels(t *testing.T) {
	tests := []struct {
		name       string
		labelPairs []string
		expected   string
	}{
		{
			name:       "empty labels",
			labelPairs: []string{},
			expected:   "",
		},
		{
			name:       "single label pair",
			labelPairs: []string{"env", "prod"},
			expected:   "env#$#prod",
		},
		{
			name:       "two label pairs sorted by key",
			labelPairs: []string{"zone", "us-east", "env", "prod"},
			expected:   "env#$#prod|zone#$#us-east",
		},
		{
			name:       "three label pairs sorted by key",
			labelPairs: []string{"c", "3", "a", "1", "b", "2"},
			expected:   "a#$#1|b#$#2|c#$#3",
		},
		{
			name:       "pipe delimiter in value is escaped",
			labelPairs: []string{"key", "val|ue"},
			expected:   "key#$#val_ue",
		},
		{
			name:       "hash-dollar delimiter in value is escaped",
			labelPairs: []string{"key", "val#$#ue"},
			expected:   "key#$#val_ue",
		},
		{
			name:       "both delimiters in value are escaped",
			labelPairs: []string{"key", "a|b#$#c"},
			expected:   "key#$#a_b_c",
		},
		{
			name:       "odd-length slice logs warning and handles gracefully",
			labelPairs: []string{"key"},
			expected:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildLabels(tt.labelPairs)
			assert.Equal(t, tt.expected, result)
		})
	}
}
