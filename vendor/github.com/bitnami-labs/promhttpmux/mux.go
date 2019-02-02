// Package promhttpmux offers an opinionated metric exporter for
// http mux/routers.
//
// The intended usage for this package is to just wrap your whole ServeMux with the
// promhttpmux.Interceptor and have it report a bunch of metrics partitioned by
// status code, http method and mux's pattern (aka route).
//
// E.g.: http_requests_total{code="200",method="GET",route="/api/v1/auth/status"} 123
//
// Usage:
//
//     http.HandleFunc("/api/v1/auth/status", statusHandler)
//     http.ListenAndServe(":8080", promhttpmux.Instrument(nil))
//
// It exports the following metrics
//
// - http_requests_total
// - http_request_duration_seconds
// - http_request_header_duration_seconds
// - http_request_size_bytes
// - http_response_size_bytes
//
// All durations are histograms because summaries are not aggregatable
// (see excellent explaination why in http://latencytipoftheday.blogspot.it/2014/06/latencytipoftheday-you-cant-average.html)
//
// Aggregation are important when you have replicas of your service.
//
// Request and response sizes are summaries because we have no idea about reasonable bucket sizes for them.
package promhttpmux

import (
	"fmt"
	"net/http"

	"github.com/felixge/httpsnoop"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	labels = []string{"code", "method", "route"}

	reqTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "http_requests_total",
			Help: "How many HTTP requests processed.",
		},
		labels,
	)

	reqDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_duration_seconds",
			Help:    "A histogram of latencies for requests.",
			Buckets: prometheus.DefBuckets,
		},
		labels,
	)

	reqHeaderDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "http_request_header_duration_seconds",
			Help:    "A histogram of latencies for emitting header.",
			Buckets: prometheus.DefBuckets,
		},
		labels,
	)

	reqSize = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "http_request_size_bytes",
			Help: "A summary of sizes for requests.",
		},
		labels,
	)

	respSize = prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Name: "http_response_size_bytes",
			Help: "A summary of sizes for responses.",
		},
		labels,
	)
)

func init() {
	prometheus.MustRegister(reqTotal)
	prometheus.MustRegister(reqDuration)
	prometheus.MustRegister(reqHeaderDuration)
	prometheus.MustRegister(reqSize)
	prometheus.MustRegister(respSize)
}

// MuxPatternResolver abstracts the fetching of the route pattern that matches a given request.
type MuxPatternResolver interface {
	Handler(r *http.Request) (http.Handler, string)
}

// Mux is a HTTP mux that knows how to resolve a pattern for a given request.
type Mux interface {
	MuxPatternResolver
	http.Handler
}

type relabelingObserverVec struct {
	prometheus.ObserverVec
	extraLabelValues prometheus.Labels
}

func (o *relabelingObserverVec) With(l prometheus.Labels) prometheus.Observer {
	for k, v := range o.extraLabelValues {
		l[k] = v
	}
	return o.ObserverVec.With(l)
}

func (o *relabelingObserverVec) Describe(ch chan<- *prometheus.Desc) {
	n := make(chan *prometheus.Desc, 1)
	o.ObserverVec.Describe(n)
	<-n // consume in case the sender is blocking

	// promhttp needs the descriptor in order to cope with metrics that have
	// either 0 labels, ["code"], or ["code", "method'] labels.
	// We have one label more and we'll fill it in in our impl of With,
	// but we need fool promhttp to believe our metric doesn't have more labels.
	ch <- prometheus.NewDesc("dummy", "dummy", []string{"code", "method"}, nil)
}

// interceptStatusCode wraps the httpsnoop API into something we can more elegantly weave in our
// interceptor chain.
func interceptStatusCode(code *int, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m := httpsnoop.CaptureMetrics(next, w, r)
		*code = m.Code
	})
}

// Instrument is a middleware that records metrics about
// http calls. It uses knowledge of the mux to add a route pattern label to each event.
func Instrument(m Mux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m == nil {
			m = http.DefaultServeMux
		}

		_, pattern := m.Handler(r)
		var statusCode int

		// Intercepting status code and size from the request is a surprisingly hard task since the http.ResponseWriter
		// can be optinally extended with a half dozen of optional extra interface such Hijaker and ReaderFrom,
		// so it's not easy to just intercept a call to Write and do the measurement.
		//
		// The excellent httpsnoop package can do most of it but the prometheus interceptors are even better:
		// e.g. they can estimate the size of the request and compute the time it takes to output a header (useful for
		// slow streaming responses such as interactive logs)
		//
		// However these interceptors don't expose the low level request/response measuring code but instead offer
		// only an API that directly feeds the metric, and they can only work with metrics that have only "code" and "method" labels.
		//
		// This package however aims at adding adding the "route" label and hence we need to intercept the observations
		// by wrapping the observers passed down to the interceptors.

		extraLabels := prometheus.Labels{"route": pattern}
		chain := promhttp.InstrumentHandlerDuration(&relabelingObserverVec{reqDuration, extraLabels},
			promhttp.InstrumentHandlerTimeToWriteHeader(&relabelingObserverVec{reqHeaderDuration, extraLabels},
				promhttp.InstrumentHandlerRequestSize(&relabelingObserverVec{reqSize, extraLabels},
					promhttp.InstrumentHandlerResponseSize(&relabelingObserverVec{respSize, extraLabels},
						interceptStatusCode(&statusCode, m)))))

		// Handlers, composition and helpers and interceptors, they all contribute to make it a little bit hard to follow
		// whats going on and how the actuall handler gets invoked. Let's try to tell it in words:
		//
		// A go http server needs just a handler. A handler is just something that implements ServeHTTP(w, r)
		// A mux is just a handler that dispatches to other handlers based on the request.
		// This interceptor takes a mux and returns a handler. You can pass this handler to the Go http server.
		// This handler will call the mux's ServeHTTP. This is all transparent.
		// The promhttp package exposes a bunch of interceptors that behave in the very same way as this.
		// We build a chain of them and at the end we have a handler, i.e. chain.ServeHTTP would cause the original
		// mux's ServeHTTP to be called.
		chain.ServeHTTP(w, r)

		// ObserverVec is an interface so we can inject our own wrapper that adds the pattern label.
		// TODO(mkm) consider contributing upstream and make CounterVec be an interface as well.
		// Not a big deal since counting is not hard and we still have httpsnoop to tell us things like the status code.
		//
		// We need the statusCode label and we get by using the interceptStatusCode middleware.
		reqTotal.WithLabelValues(fmt.Sprint(statusCode), r.Method, pattern).Inc()
	})
}
