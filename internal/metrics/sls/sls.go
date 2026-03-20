// Package sls provides a push-mode metrics exporter for Alibaba Cloud Log Service (SLS).
package sls

import (
	"context"
	"errors"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	sls "github.com/aliyun/aliyun-log-go-sdk"
	"github.com/aliyun/aliyun-log-go-sdk/producer"
	"github.com/FZambia/eagle"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
)

// ptrUint32 and ptrString replace proto.Uint32/proto.String to avoid a direct
// dependency on the deprecated github.com/golang/protobuf package.
func ptrUint32(v uint32) *uint32 { return &v }
func ptrString(v string) *string { return &v }

const producerCloseTimeoutMs = 10000

// Exporter pushes Prometheus metrics to Alibaba Cloud Log Service (SLS).
type Exporter struct {
	project  string
	logstore string
	topic    string
	source   string
	// fcLabelPairs holds the pre-sanitised FC label key/value pairs
	// ["instance_id", "<val>", "version", "<val>"] read once at startup from
	// FC_INSTANCE_ID and FC_FUNCTION_VERSION. Pairs whose env var is empty are
	// omitted. Included in every metric's sorted __labels__ field.
	fcLabelPairs []string
	closeOnce    sync.Once
	closeCh      chan struct{}
	sink         chan eagle.Metrics
	eagle        *eagle.Eagle
	producer     *producer.Producer
}

// Config for SLS Exporter.
// Alibaba Cloud credentials are NOT part of the config; they are read from the
// following environment variables at startup (values never change at runtime):
//   - ALIBABA_CLOUD_ACCESS_KEY_ID     — AccessKey ID
//   - ALIBABA_CLOUD_ACCESS_KEY_SECRET — AccessKey Secret
//   - ALIBABA_CLOUD_SECURITY_TOKEN    — STS security token (optional)
type Config struct {
	// Gatherer is the Prometheus gatherer used to collect metrics.
	Gatherer prometheus.Gatherer
	// Interval is the interval between metric pushes.
	Interval time.Duration
	// Endpoint is the SLS service endpoint (e.g. https://cn-hangzhou.log.aliyuncs.com).
	Endpoint string
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
// Credentials are read once from environment variables:
//   - ALIBABA_CLOUD_ACCESS_KEY_ID, ALIBABA_CLOUD_ACCESS_KEY_SECRET, ALIBABA_CLOUD_SECURITY_TOKEN
//
// Extra labels are read once from FC_INSTANCE_ID and FC_FUNCTION_VERSION.
func New(c Config) (*Exporter, error) {
	accessKeyID := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_ID")
	accessKeySecret := os.Getenv("ALIBABA_CLOUD_ACCESS_KEY_SECRET")
	securityToken := os.Getenv("ALIBABA_CLOUD_SECURITY_TOKEN")

	if accessKeyID == "" || accessKeySecret == "" {
		return nil, errors.New("aliyun_sls: ALIBABA_CLOUD_ACCESS_KEY_ID and ALIBABA_CLOUD_ACCESS_KEY_SECRET environment variables are required")
	}

	producerConfig := producer.GetDefaultProducerConfig()
	producerConfig.Endpoint = c.Endpoint
	producerConfig.CredentialsProvider = sls.NewStaticCredentialsProvider(accessKeyID, accessKeySecret, securityToken)
	producerInstance, err := producer.NewProducer(producerConfig)
	if err != nil {
		return nil, err
	}

	// Build FC label pairs once from env vars (immutable at runtime).
	// Keys and values are pre-sanitised so no per-metric escaping is needed.
	var fcLabelPairs []string
	if id := os.Getenv("FC_INSTANCE_ID"); id != "" {
		fcLabelPairs = append(fcLabelPairs, "instance_id", labelValueReplacer.Replace(id))
	}
	if ver := os.Getenv("FC_FUNCTION_VERSION"); ver != "" {
		fcLabelPairs = append(fcLabelPairs, "version", labelValueReplacer.Replace(ver))
	}

	sink := make(chan eagle.Metrics)
	exporter := &Exporter{
		project:      c.Project,
		logstore:     c.Logstore,
		topic:        c.Topic,
		source:       c.Source,
		fcLabelPairs: fcLabelPairs,
		closeCh:      make(chan struct{}),
		sink:         sink,
		producer:     producerInstance,
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
			labels := buildLabels(metricValue.Labels, e.fcLabelPairs)
			slsLog := buildLog(now, nowNano, metricName, labels, metricValue.Value)
			if err := e.producer.SendLog(e.project, e.logstore, e.topic, e.source, slsLog); err != nil {
				log.Error().Err(err).Str("metric", metricName).Msg("error sending metric to Alibaba Cloud SLS")
			}
		}
	}
}

func buildMetricName(item eagle.Metric, metricValue eagle.MetricValue) string {
	var sb strings.Builder
	if item.Namespace != "" {
		sb.WriteString(item.Namespace)
	}
	if item.Subsystem != "" {
		if sb.Len() > 0 {
			sb.WriteByte('_')
		}
		sb.WriteString(item.Subsystem)
	}
	if item.Name != "" {
		if sb.Len() > 0 {
			sb.WriteByte('_')
		}
		sb.WriteString(item.Name)
	}
	if metricValue.Name != "" {
		if sb.Len() > 0 {
			sb.WriteByte('_')
		}
		sb.WriteString(metricValue.Name)
	}
	return sb.String()
}

// labelValueReplacer replaces SLS label delimiter sequences in label keys and
// values to avoid ambiguity when parsing the encoded label string. Both "|" and
// "#$#" are replaced with "_". Prometheus label names are restricted to
// [a-zA-Z_][a-zA-Z0-9_]* and cannot normally contain these sequences; the
// replacer is applied to keys as well to be safe by construction.
var labelValueReplacer = strings.NewReplacer("|", "_", "#$#", "_")

// buildLabels converts a flat label slice (alternating key/value pairs) plus the
// pre-cached FC label pairs into the SLS MetricStore __labels__ field value:
// "k1#$#v1|k2#$#v2". All labels are sorted alphabetically by key as required
// by the SLS MetricStore write protocol. Delimiter sequences ("|" and "#$#") in
// label keys and values are replaced with "_".
func buildLabels(labelPairs []string, fcLabelPairs []string) string {
	if len(labelPairs)%2 != 0 {
		log.Warn().Int("len", len(labelPairs)).Msg("sls: odd number of label pair elements, last element ignored")
	}
	n := len(labelPairs)/2 + len(fcLabelPairs)/2
	if n == 0 {
		return ""
	}

	type kv struct{ k, v string }
	pairs := make([]kv, 0, n)
	for i := 0; i+1 < len(labelPairs); i += 2 {
		pairs = append(pairs, kv{
			k: labelValueReplacer.Replace(labelPairs[i]),
			v: labelValueReplacer.Replace(labelPairs[i+1]),
		})
	}
	for i := 0; i+1 < len(fcLabelPairs); i += 2 {
		// FC pairs are already sanitised at startup.
		pairs = append(pairs, kv{k: fcLabelPairs[i], v: fcLabelPairs[i+1]})
	}

	sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })

	var sb strings.Builder
	sb.WriteString(pairs[0].k)
	sb.WriteString("#$#")
	sb.WriteString(pairs[0].v)
	for _, p := range pairs[1:] {
		sb.WriteByte('|')
		sb.WriteString(p.k)
		sb.WriteString("#$#")
		sb.WriteString(p.v)
	}
	return sb.String()
}

func buildLog(now uint32, nowNano, metricName, labels string, value float64) *sls.Log {
	slsLog := &sls.Log{Time: ptrUint32(now)}
	contents := []*sls.LogContent{
		{Key: ptrString("__time_nano__"), Value: ptrString(nowNano)},
		{Key: ptrString("__name__"), Value: ptrString(metricName)},
		{Key: ptrString("__value__"), Value: ptrString(strconv.FormatFloat(value, 'f', 6, 64))},
		{Key: ptrString("__labels__"), Value: ptrString(labels)},
	}
	slsLog.Contents = contents
	return slsLog
}
