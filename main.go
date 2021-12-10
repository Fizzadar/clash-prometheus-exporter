package main

import (
	"encoding/json"
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
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
)

var u url.URL
var client http.Client

type Connection struct {
	Id       string `json:"id"`
	Upload   int    `json:"upload"`
	Download int    `json:"download"`
}

type ConnectionsResponse struct {
	DownloadTotal int `json:"downloadTotal"`
	UploadTotal   int `json:"uploadTotal"`
	Connections   []Connection
}

var (
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

	for _, connection := range response.Connections {
		connectionDownloadGauges.WithLabelValues(connection.Id).Set(float64(connection.Download))
		connectionUploadGauges.WithLabelValues(connection.Id).Set(float64(connection.Upload))
	}
}

func collectMetricsLoop() {
	u = url.URL{Scheme: "http", Host: *clashAddr, Path: "/connections"}
	log.Printf("connecting to %s", u.String())

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

	go collectMetricsLoop()

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}
