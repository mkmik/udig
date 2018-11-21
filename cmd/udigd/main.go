package main // imports "github.com/bitnami-labs/udig/cmd/udigd"

import (
	"flag"
	"net"
	"net/http"

	// registers debug handlers
	_ "net/http/pprof"

	"github.com/bitnami-labs/promhttpmux"
	"github.com/cockroachdb/cmux"
	"github.com/golang/glog"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	forwarded "github.com/stanvit/go-forwarded"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	laddr = flag.String("listen", ":4000", "http/grpc listening address:port")
)

func run(laddr string) error {
	grpc.EnableTracing = true
	grpc_prometheus.EnableHandlingTimeHistogram()
	trace.AuthRequest = func(*http.Request) (bool, bool) { return true, true }

	mux := http.DefaultServeMux

	lis, err := net.Listen("tcp", laddr)
	if err != nil {
		return errors.Trace(err)
	}

	m := cmux.New(lis)

	grpcL := m.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
	httpL := m.Match(cmux.HTTP1Fast())

	gs := grpc.NewServer()
	reflection.Register(gs)
	grpc_prometheus.Register(gs)
	mux.Handle("/metrics", promhttp.Handler())

	// Make r.RemoteAddr honour the X-Forwarded-For header set by our load balancer (k8s ingress).
	clientIPWrapper, _ := forwarded.New("0.0.0.0/0", false, false, "X-Forwarded-For", "X-Forwarded-Protocol")

	// Actually serve gRPC and HTTP
	go http.Serve(httpL, clientIPWrapper.Handler(promhttpmux.Instrument(mux)))
	go gs.Serve(grpcL)

	glog.Infof("serving gRPC and http endpoint on %s", lis.Addr())
	// Serve the multiplexer and block
	return errors.Trace(m.Serve())
}

func main() {
	flag.Parse()
	defer glog.Flush()

	if err := run(*laddr); err != nil {
		glog.Fatalf("%+v", err)
	}
}
