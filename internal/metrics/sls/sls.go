// Package sls provides a push-mode metrics exporter for Alibaba Cloud Log Service (SLS).
package sls

import (
	"context"
	"errors"
	"os"
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
	project          string
	logstore         string
	topic            string
	source           string
	// extraLabelSuffix is a pre-formatted "|k1#$#v1|k2#$#v2" string that is
	// appended to every metric's label string. It is built once at construction
	// time from FC_INSTANCE_ID and FC_FUNCTION_VERSION env vars.
	extraLabelSuffix string
	closeOnce        sync.Once
	closeCh          chan struct{}
	sink             chan eagle.Metrics
	eagle            *eagle.Eagle
	producer         *producer.Producer
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

	// Build the extra label suffix once from FC env vars (immutable at runtime).
	extraLabelSuffix := buildExtraLabelSuffix()

	sink := make(chan eagle.Metrics)
	exporter := &Exporter{
		project:          c.Project,
		logstore:         c.Logstore,
		topic:            c.Topic,
		source:           c.Source,
		extraLabelSuffix: extraLabelSuffix,
		closeCh:          make(chan struct{}),
		sink:             sink,
		producer:         producerInstance,
	}
	exporter.eagle = eagle.New(eagle.Config{
		Gatherer: c.Gatherer,
		Interval: c.Interval,
		Sink:     sink,
	})
	return exporter, nil
}

// buildExtraLabelSuffix builds the pre-formatted extra label string from FC env
// vars.  It returns a string of the form "|instance_id#$#<val>|version#$#<val>"
// (with only the pairs whose env var is non-empty included).
func buildExtraLabelSuffix() string {
	instanceID := os.Getenv("FC_INSTANCE_ID")
	functionVersion := os.Getenv("FC_FUNCTION_VERSION")
	if instanceID == "" && functionVersion == "" {
		return ""
	}
	var sb strings.Builder
	if instanceID != "" {
		sb.WriteString("|instance_id#$#")
		sb.WriteString(labelValueReplacer.Replace(instanceID))
	}
	if functionVersion != "" {
		sb.WriteString("|version#$#")
		sb.WriteString(labelValueReplacer.Replace(functionVersion))
	}
	return sb.String()
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
			labels := buildLabels(metricValue.Labels, e.extraLabelSuffix)
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

// buildLabels converts a flat label slice (alternating key/value pairs) into
// the SLS time series label format: "k1#$#v1|k2#$#v2".
// Delimiter sequences ("|" and "#$#") in label keys and values are replaced
// with "_" to prevent ambiguity during SLS parsing. Labels are emitted in
// input order; SLS does not require sorted label keys.
// extraSuffix is a pre-formatted "|k#$#v..." string appended verbatim at the end.
func buildLabels(labelPairs []string, extraSuffix string) string {
	if len(labelPairs) < 2 && extraSuffix == "" {
		return ""
	}
	if len(labelPairs)%2 != 0 {
		log.Warn().Int("len", len(labelPairs)).Msg("sls: odd number of label pair elements, last element ignored")
	}
	var sb strings.Builder
	if len(labelPairs) >= 2 {
		sb.WriteString(labelValueReplacer.Replace(labelPairs[0]))
		sb.WriteString("#$#")
		sb.WriteString(labelValueReplacer.Replace(labelPairs[1]))
		for i := 2; i+1 < len(labelPairs); i += 2 {
			sb.WriteByte('|')
			sb.WriteString(labelValueReplacer.Replace(labelPairs[i]))
			sb.WriteString("#$#")
			sb.WriteString(labelValueReplacer.Replace(labelPairs[i+1]))
		}
	}
	if extraSuffix != "" {
		if sb.Len() > 0 {
			// extraSuffix already starts with "|"
			sb.WriteString(extraSuffix)
		} else if len(extraSuffix) > 1 {
			// no base labels — strip the leading "|"
			sb.WriteString(extraSuffix[1:])
		}
	}
	return sb.String()
}

func buildLog(now uint32, nowNano, metricName, labels string, value float64) *sls.Log {
	slsLog := &sls.Log{Time: ptrUint32(now)}
	contents := []*sls.LogContent{
		{Key: ptrString("time_nano"), Value: ptrString(nowNano)},
		{Key: ptrString("name"), Value: ptrString(metricName)},
		{Key: ptrString("value"), Value: ptrString(strconv.FormatFloat(value, 'f', 6, 64))},
		{Key: ptrString("labels"), Value: ptrString(labels)},
	}
	slsLog.Contents = contents
	return slsLog
}
