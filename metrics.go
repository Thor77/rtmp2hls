package main

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	log "github.com/sirupsen/logrus"
)

const namespace = "rtmp2hls"

var totalConnections = promauto.NewCounter(prometheus.CounterOpts{
	Namespace: namespace,
	Name:      "connections_total",
	Help:      "total number of connections",
})
var currentConnections = promauto.NewGauge(prometheus.GaugeOpts{
	Namespace: namespace,
	Name:      "connections_current",
	Help:      "current number of connections",
})
var totalErrors = promauto.NewCounterVec(prometheus.CounterOpts{
	Namespace: namespace,
	Name:      "errors_total",
	Help:      "total number of errors",
}, []string{"error"})

func serveMetrics() {
	metricsAddr := ":2112"
	http.Handle("/metrics", promhttp.Handler())
	log.Infof("Metrics handler listening on %s\n", metricsAddr)
	http.ListenAndServe(metricsAddr, nil)
}
