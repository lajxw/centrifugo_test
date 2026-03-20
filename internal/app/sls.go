package app

import (
	"github.com/centrifugal/centrifugo/v6/internal/config"
	slsexporter "github.com/centrifugal/centrifugo/v6/internal/metrics/sls"

	"github.com/prometheus/client_golang/prometheus"
)

func aliyunSLSExporter(cfg config.Config) (*slsexporter.Exporter, error) {
	return slsexporter.New(slsexporter.Config{
		Gatherer:        prometheus.DefaultGatherer,
		Interval:        cfg.AliyunSLS.Interval.ToDuration(),
		Endpoint:        cfg.AliyunSLS.Endpoint,
		AccessKeyID:     cfg.AliyunSLS.AccessKeyID,
		AccessKeySecret: cfg.AliyunSLS.AccessKeySecret,
		Project:         cfg.AliyunSLS.Project,
		Logstore:        cfg.AliyunSLS.Logstore,
		Topic:           cfg.AliyunSLS.Topic,
		Source:          cfg.AliyunSLS.Source,
	})
}
