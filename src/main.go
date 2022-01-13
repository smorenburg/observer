package main

import (
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var logFatal = log.Fatal
var logPrintf = log.Printf
var sleep = time.Sleep
var serviceName = "observer"

var prometheusHandler = func() http.Handler {
	return promhttp.Handler()
}

var (
	histogram = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Subsystem: "http_server",
		Name:      "resp_time",
		Help:      "Request response time",
	}, []string{
		"service",
		"code",
		"method",
		"path",
	})
)

func init() {
	prometheus.MustRegister(histogram)
}

func main() {
	if len(os.Getenv("SERVICE_NAME")) > 0 {
		serviceName = os.Getenv("SERVICE_NAME")
	}
	runServer()
}

func runServer() {
	mux := http.NewServeMux()
	mux.HandleFunc("/", indexServer)
	mux.HandleFunc("/random-delay", randomDelayServer)
	mux.HandleFunc("/random-error", randomErrorServer)

	muxMetrics := http.NewServeMux()
	muxMetrics.Handle("/metrics", prometheusHandler())

	go func() {
		logFatal("ListenAndServe: ", http.ListenAndServe(":9090", muxMetrics))
	}()

	logFatal("ListenAndServe: ", http.ListenAndServe(":8080", mux))
}

func indexServer(w http.ResponseWriter, req *http.Request) {
	code := http.StatusOK
	start := time.Now()
	defer func() { recordMetrics(start, req, code) }()

	logPrintf("%s request to %s\n", req.Method, req.RequestURI)

	if req.Method != "GET" {
		code = http.StatusNotFound
		http.Error(w, "Method is not supported.", code)
		return
	}

	_, err := io.WriteString(w, "Hello, world!\n")
	if err != nil {
		return
	}
}

func randomDelayServer(w http.ResponseWriter, req *http.Request) {
	code := http.StatusOK
	start := time.Now()
	defer func() { recordMetrics(start, req, code) }()

	logPrintf("%s request to %s\n", req.Method, req.RequestURI)

	if req.Method != "GET" {
		code = http.StatusNotFound
		http.Error(w, "Method is not supported.", code)
		return
	}

	rand.Seed(time.Now().UnixNano())
	n := rand.Intn(1000)
	sleep(time.Duration(n) * time.Millisecond)
	msg := fmt.Sprintf("Hello world! Delayed for %d ms.\n", n)

	_, err := io.WriteString(w, msg)
	if err != nil {
		return
	}
}

func randomErrorServer(w http.ResponseWriter, req *http.Request) {
	code := http.StatusOK
	start := time.Now()
	defer func() { recordMetrics(start, req, code) }()

	logPrintf("%s request to %s\n", req.Method, req.RequestURI)

	if req.Method != "GET" {
		code = http.StatusNotFound
		http.Error(w, "Method is not supported.", code)
		return
	}

	rand.Seed(time.Now().UnixNano())
	n := rand.Intn(10)
	msg := "Hello, world!\n"
	if n == 0 {
		code = http.StatusInternalServerError
		msg = "error: Something, somewhere, went wrong!\n"
		logPrintf(msg)
	}

	w.WriteHeader(code)
	_, err := io.WriteString(w, msg)
	if err != nil {
		return
	}
}

func recordMetrics(start time.Time, req *http.Request, code int) {
	duration := time.Since(start)
	histogram.With(
		prometheus.Labels{
			"service": serviceName,
			"code":    fmt.Sprintf("%d", code),
			"method":  req.Method,
			"path":    req.URL.Path,
		},
	).Observe(duration.Seconds())
}
