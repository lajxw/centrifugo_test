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
		name         string
		labelPairs   []string
		fcLabelPairs []string
		expected     string
	}{
		{
			name:       "empty labels no extra",
			labelPairs: []string{},
			expected:   "",
		},
		{
			name:         "empty labels with fc label pairs",
			labelPairs:   []string{},
			fcLabelPairs: []string{"instance_id", "i-123", "version", "v1"},
			expected:     "instance_id#$#i-123|version#$#v1",
		},
		{
			name:       "single label pair",
			labelPairs: []string{"env", "prod"},
			expected:   "env#$#prod",
		},
		{
			name:         "single label pair with fc label pair",
			labelPairs:   []string{"env", "prod"},
			fcLabelPairs: []string{"instance_id", "i-123"},
			expected:     "env#$#prod|instance_id#$#i-123",
		},
		{
			name:       "two label pairs sorted alphabetically",
			labelPairs: []string{"zone", "us-east", "env", "prod"},
			expected:   "env#$#prod|zone#$#us-east",
		},
		{
			name:       "three label pairs sorted alphabetically",
			labelPairs: []string{"c", "3", "a", "1", "b", "2"},
			expected:   "a#$#1|b#$#2|c#$#3",
		},
		{
			name:       "delimiter in key is escaped",
			labelPairs: []string{"ke#$#y", "val"},
			expected:   "ke_y#$#val",
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
			name:       "odd-length slice (len=3) drops last element",
			labelPairs: []string{"a", "1", "orphan"},
			expected:   "a#$#1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildLabels(tt.labelPairs, tt.fcLabelPairs)
			assert.Equal(t, tt.expected, result)
		})
	}
}


