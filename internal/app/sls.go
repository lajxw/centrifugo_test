package app

import (
	"errors"

	"github.com/centrifugal/centrifugo/v6/internal/config"
	slsexporter "github.com/centrifugal/centrifugo/v6/internal/metrics/sls"

	"github.com/prometheus/client_golang/prometheus"
)

func aliyunSLSExporter(cfg config.Config) (*slsexporter.Exporter, error) {
	c := cfg.AliyunSLS
	if c.Endpoint == "" || c.AccessKeyID == "" || c.AccessKeySecret == "" || c.Project == "" || c.Logstore == "" {
		return nil, errors.New("aliyun_sls: endpoint, access_key_id, access_key_secret, project, and logstore are required")
	}
	return slsexporter.New(slsexporter.Config{
		Gatherer:        prometheus.DefaultGatherer,
		Interval:        c.Interval.ToDuration(),
		Endpoint:        c.Endpoint,
		AccessKeyID:     c.AccessKeyID,
		AccessKeySecret: c.AccessKeySecret,
		Project:         c.Project,
		Logstore:        c.Logstore,
		Topic:           c.Topic,
		Source:          c.Source,
	})
}
