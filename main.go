package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	listenAddr = flag.String(
		"listen-address",
		"127.0.0.1:9869",
		"Address to listen on",
	)
	clashAddr = flag.String(
		"clash-address",
		"127.0.0.1:9090",
		"Address of the clash API",
	)
	clashTimeout = flag.Duration(
		"clash-timeout",
		5*time.Second,
		"Timeout for reading from the clash API",
	)
	collectInterval = flag.Duration(
		"collect-interval",
		30*time.Second,
		"Interval to collect metrics from clash",
	)
	metricsPath = flag.String(
		"metrics-path",
		"/metrics",
		"Path to serve metrics at",
	)
)

var (
	u      url.URL
	client http.Client
)

type Connection struct {
	Id       string   `json:"id"`
	Upload   int      `json:"upload"`
	Download int      `json:"download"`
	Chains   []string `json:"chains"`
}

type ConnectionsResponse struct {
	DownloadTotal int `json:"downloadTotal"`
	UploadTotal   int `json:"uploadTotal"`
	Connections   []Connection
}

type ChainMetrics struct {
	ConnectionCount int
	DownloadTotal   int
	UploadTotal     int
}

func (metrics *ChainMetrics) addConnection() {
	metrics.ConnectionCount += 1
}

func (metrics *ChainMetrics) addDownload(download int) {
	metrics.DownloadTotal += download
}

func (metrics *ChainMetrics) addUpload(upload int) {
	metrics.UploadTotal += upload
}

var (
	// Global/instance level metrics
	connectionsGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "clash",
		Name:      "connections",
		Help:      "Number of current connections.",
	})
	totalDownloadGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "clash",
		Name:      "download_bytes",
		Help:      "Total data downloaded in bytes.",
	})
	totalUploadGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Namespace: "clash",
		Name:      "upload_bytes",
		Help:      "Total data uploaded in bytes.",
	})

	// Chain (upstream proxy) level metrics
	chainConnectionGauges = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "clash",
			Subsystem: "chain",
			Name:      "connections",
			Help:      "Number of current connections per proxy chain.",
		},
		[]string{"chain"},
	)
	chainDownloadGauges = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "clash",
			Subsystem: "chain",
			Name:      "download_bytes",
			Help:      "Total data downloaded in bytes per proxy chain.",
		},
		[]string{"chain"},
	)
	chainUploadGauges = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "clash",
			Subsystem: "chain",
			Name:      "upload_bytes",
			Help:      "Total data uploaded in bytes per proxy chain.",
		},
		[]string{"chain"},
	)

	// Conenction level metrics
	connectionDownloadGauges = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "clash",
			Subsystem: "connection",
			Name:      "download_bytes",
			Help:      "Total data uploaded in bytes per connection.",
		},
		[]string{"id"},
	)
	connectionUploadGauges = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: "clash",
			Subsystem: "connection",
			Name:      "upload_bytes",
			Help:      "Total data uploaded in bytes per connection.",
		},
		[]string{"id"},
	)
)

func collectMetrics() {
	// Clear any existing metrics since we refetch them all
	chainConnectionGauges.Reset()
	chainDownloadGauges.Reset()
	chainUploadGauges.Reset()
	connectionDownloadGauges.Reset()
	connectionUploadGauges.Reset()

	req, reqErr := http.NewRequest(http.MethodGet, u.String(), nil)
	if reqErr != nil {
		log.Fatal(reqErr)
	}

	res, getErr := client.Do(req)
	if getErr != nil {
		log.Printf("Error fetching connections from clash: %s", getErr)
		return
	}

	if res.Body != nil {
		defer res.Body.Close()
	}

	body, readErr := ioutil.ReadAll(res.Body)
	if readErr != nil {
		log.Fatal(readErr)
	}

	response := ConnectionsResponse{}
	jsonErr := json.Unmarshal(body, &response)
	if jsonErr != nil {
		log.Fatal(jsonErr)
	}

	connectionsGauge.Set(float64(len(response.Connections)))
	totalDownloadGauge.Set(float64(response.DownloadTotal))
	totalUploadGauge.Set(float64(response.UploadTotal))

	chainToMetrics := make(map[string]*ChainMetrics)

	for _, connection := range response.Connections {
		connectionDownloadGauges.WithLabelValues(connection.Id).Set(float64(connection.Download))
		connectionUploadGauges.WithLabelValues(connection.Id).Set(float64(connection.Upload))

		var chainKey string = strings.Join(connection.Chains, ",")

		chainMetrics, exists := chainToMetrics[chainKey]
		if exists {
			chainMetrics.addConnection()
			chainMetrics.addDownload(connection.Download)
			chainMetrics.addUpload(connection.Upload)
		} else {
			chainToMetrics[chainKey] = &ChainMetrics{
				ConnectionCount: 1,
				DownloadTotal:   connection.Download,
				UploadTotal:     connection.Upload,
			}
		}
	}

	for chainKey, chainMetrics := range chainToMetrics {
		chainConnectionGauges.WithLabelValues(chainKey).Set(float64(chainMetrics.ConnectionCount))
		chainDownloadGauges.WithLabelValues(chainKey).Set(float64(chainMetrics.DownloadTotal))
		chainUploadGauges.WithLabelValues(chainKey).Set(float64(chainMetrics.UploadTotal))
	}
}

func collectMetricsLoop() {
	u = url.URL{Scheme: "http", Host: *clashAddr, Path: "/connections"}
	log.Printf("Connecting to %s", u.String())

	client = http.Client{
		Timeout: *clashTimeout,
	}

	for {
		collectMetrics()
		time.Sleep(*collectInterval)
	}
}

func main() {
	flag.Parse()

	prometheus.MustRegister(connectionsGauge)
	prometheus.MustRegister(totalDownloadGauge)
	prometheus.MustRegister(totalUploadGauge)
	prometheus.MustRegister(connectionDownloadGauges)
	prometheus.MustRegister(connectionUploadGauges)
	prometheus.MustRegister(chainConnectionGauges)
	prometheus.MustRegister(chainDownloadGauges)
	prometheus.MustRegister(chainUploadGauges)

	go collectMetricsLoop()

	http.Handle(*metricsPath, promhttp.Handler())
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>
			<head><title>Clash Prometheus Exporter</title></head>
			<body>
			<h1>Clash Prometheus Exporter</h1>
			<p><a href='` + *metricsPath + `'>Metrics</a></p>
			</body>
			</html>`))
	})

	log.Printf("Starting listen on http://%s", *listenAddr)
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}
