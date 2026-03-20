// Package sls provides a push-mode metrics exporter for Alibaba Cloud Log Service (SLS).
package sls

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sls "github.com/aliyun/aliyun-log-go-sdk"
	"github.com/aliyun/aliyun-log-go-sdk/producer"
	"github.com/FZambia/eagle"
	"github.com/golang/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

const producerCloseTimeoutMs = 10000

// Exporter pushes Prometheus metrics to Alibaba Cloud Log Service (SLS).
type Exporter struct {
	project  string
	logstore string
	topic    string
	source   string
	closeOnce sync.Once
	closeCh  chan struct{}
	sink     chan eagle.Metrics
	eagle    *eagle.Eagle
	producer *producer.Producer
}

// Config for SLS Exporter.
type Config struct {
	// Gatherer is the Prometheus gatherer used to collect metrics.
	Gatherer prometheus.Gatherer
	// Interval is the interval between metric pushes.
	Interval time.Duration
	// Endpoint is the SLS service endpoint (e.g. https://cn-hangzhou.log.aliyuncs.com).
	Endpoint string
	// AccessKeyID is the Alibaba Cloud AccessKey ID.
	AccessKeyID string
	// AccessKeySecret is the Alibaba Cloud AccessKey Secret.
	AccessKeySecret string
	// Project is the SLS project name.
	Project string
	// Logstore is the SLS logstore name.
	Logstore string
	// Topic is the SLS log topic.
	Topic string
	// Source is the log source identifier.
	Source string
}

// New creates a new SLS Exporter.
func New(c Config) (*Exporter, error) {
	producerConfig := producer.GetDefaultProducerConfig()
	producerConfig.Endpoint = c.Endpoint
	producerConfig.AccessKeyID = c.AccessKeyID       //nolint:staticcheck // SLS SDK v0.1.x uses deprecated field names
	producerConfig.AccessKeySecret = c.AccessKeySecret //nolint:staticcheck // SLS SDK v0.1.x uses deprecated field names
	producerInstance, err := producer.NewProducer(producerConfig)
	if err != nil {
		return nil, err
	}

	sink := make(chan eagle.Metrics)
	exporter := &Exporter{
		project:  c.Project,
		logstore: c.Logstore,
		topic:    c.Topic,
		source:   c.Source,
		closeCh:  make(chan struct{}),
		sink:     sink,
		producer: producerInstance,
	}
	exporter.eagle = eagle.New(eagle.Config{
		Gatherer: c.Gatherer,
		Interval: c.Interval,
		Sink:     sink,
	})
	return exporter, nil
}

// Run starts the exporter and blocks until ctx is done.
func (e *Exporter) Run(ctx context.Context) error {
	e.producer.Start()
	defer func() { _ = e.close() }()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case metrics := <-e.sink:
			e.exportOnce(metrics)
		}
	}
}

func (e *Exporter) close() error {
	e.closeOnce.Do(func() {
		close(e.closeCh)
		_ = e.eagle.Close()
		_ = e.producer.Close(producerCloseTimeoutMs)
	})
	return nil
}

func (e *Exporter) exportOnce(metrics eagle.Metrics) {
	t := time.Now()
	now := uint32(t.Unix())
	nowNano := strconv.FormatInt(t.UnixNano(), 10)

	for _, item := range metrics.Items {
		for _, metricValue := range item.Values {
			metricName := buildMetricName(item, metricValue)
			labels := buildLabels(metricValue.Labels)
			slsLog := buildLog(now, nowNano, metricName, labels, metricValue.Value)
			if err := e.producer.SendLog(e.project, e.logstore, e.topic, e.source, slsLog); err != nil {
				log.Error().Err(err).Str("metric", metricName).Msg("error sending metric to Alibaba Cloud SLS")
			}
		}
	}
}

func buildMetricName(item eagle.Metric, metricValue eagle.MetricValue) string {
	parts := make([]string, 0, 4)
	if item.Namespace != "" {
		parts = append(parts, item.Namespace)
	}
	if item.Subsystem != "" {
		parts = append(parts, item.Subsystem)
	}
	if item.Name != "" {
		parts = append(parts, item.Name)
	}
	if metricValue.Name != "" {
		parts = append(parts, metricValue.Name)
	}
	return strings.Join(parts, "_")
}

// labelValueReplacer replaces SLS label delimiter sequences in label values
// to avoid ambiguity when parsing the encoded label string. Both "|" and "#$#"
// are replaced with "_". Note that this means a value containing underscores
// and a value that originally contained these delimiters will produce the same
// output; this is an acceptable trade-off since Prometheus label values
// rarely contain these sequences.
var labelValueReplacer = strings.NewReplacer("|", "_", "#$#", "_")

// buildLabels converts a flat label slice (alternating key/value pairs) into
// the SLS time series label format: "k1#$#v1|k2#$#v2".
// Delimiter sequences ("|" and "#$#") in label values are replaced with "_"
// to prevent ambiguity during SLS parsing.
func buildLabels(labelPairs []string) string {
	if len(labelPairs) < 2 {
		return ""
	}
	if len(labelPairs)%2 != 0 {
		log.Warn().Int("len", len(labelPairs)).Msg("sls: odd number of label pair elements, last element ignored")
	}
	type kv struct{ k, v string }
	pairs := make([]kv, 0, len(labelPairs)/2)
	for i := 0; i+1 < len(labelPairs); i += 2 {
		pairs = append(pairs, kv{labelPairs[i], labelValueReplacer.Replace(labelPairs[i+1])})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })

	var sb strings.Builder
	for i, p := range pairs {
		if i > 0 {
			sb.WriteByte('|')
		}
		sb.WriteString(p.k)
		sb.WriteString("#$#")
		sb.WriteString(p.v)
	}
	return sb.String()
}

func buildLog(now uint32, nowNano, metricName, labels string, value float64) *sls.Log {
	slsLog := &sls.Log{Time: proto.Uint32(now)}
	contents := []*sls.LogContent{
		{Key: proto.String("time_nano"), Value: proto.String(nowNano)},
		{Key: proto.String("name"), Value: proto.String(metricName)},
		{Key: proto.String("value"), Value: proto.String(strconv.FormatFloat(value, 'f', 6, 64))},
		{Key: proto.String("labels"), Value: proto.String(labels)},
	}
	slsLog.Contents = contents
	return slsLog
}
