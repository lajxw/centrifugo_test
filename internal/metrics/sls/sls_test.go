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
		name        string
		labelPairs  []string
		extraSuffix string
		expected    string
	}{
		{
			name:       "empty labels no extra",
			labelPairs: []string{},
			expected:   "",
		},
		{
			name:        "empty labels with extra suffix",
			labelPairs:  []string{},
			extraSuffix: "|instance_id#$#i-123|version#$#v1",
			expected:    "instance_id#$#i-123|version#$#v1",
		},
		{
			name:       "single label pair",
			labelPairs: []string{"env", "prod"},
			expected:   "env#$#prod",
		},
		{
			name:        "single label pair with extra suffix",
			labelPairs:  []string{"env", "prod"},
			extraSuffix: "|instance_id#$#i-123",
			expected:    "env#$#prod|instance_id#$#i-123",
		},
		{
			name:       "two label pairs in input order",
			labelPairs: []string{"zone", "us-east", "env", "prod"},
			expected:   "zone#$#us-east|env#$#prod",
		},
		{
			name:       "three label pairs in input order",
			labelPairs: []string{"c", "3", "a", "1", "b", "2"},
			expected:   "c#$#3|a#$#1|b#$#2",
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
			name:       "odd-length slice (len=3) logs warning and drops last element",
			labelPairs: []string{"a", "1", "orphan"},
			expected:   "a#$#1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildLabels(tt.labelPairs, tt.extraSuffix)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBuildExtraLabelSuffix(t *testing.T) {
	tests := []struct {
		name            string
		instanceID      string
		functionVersion string
		expected        string
	}{
		{
			name:     "both empty",
			expected: "",
		},
		{
			name:       "instance_id only",
			instanceID: "i-abc123",
			expected:   "|instance_id#$#i-abc123",
		},
		{
			name:            "version only",
			functionVersion: "LATEST",
			expected:        "|version#$#LATEST",
		},
		{
			name:            "both set",
			instanceID:      "i-abc123",
			functionVersion: "v2",
			expected:        "|instance_id#$#i-abc123|version#$#v2",
		},
		{
			name:            "delimiter in value is escaped",
			instanceID:      "i|abc",
			functionVersion: "v#$#1",
			expected:        "|instance_id#$#i_abc|version#$#v_1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("FC_INSTANCE_ID", tt.instanceID)
			t.Setenv("FC_FUNCTION_VERSION", tt.functionVersion)
			result := buildExtraLabelSuffix()
			assert.Equal(t, tt.expected, result)
		})
	}
}

