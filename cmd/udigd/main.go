package main // imports "github.com/bitnami-labs/udig/cmd/udigd"

import (
	"context"
	"flag"
	"net"
	"net/http"
	"time"

	// registers debug handlers
	_ "net/http/pprof"

	"github.com/bitnami-labs/promhttpmux"
	"github.com/bitnami-labs/udig/pkg/uplink/uplinkpb"
	"github.com/golang/glog"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/hashicorp/yamux"
	"github.com/juju/errors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	forwarded "github.com/stanvit/go-forwarded"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
)

var (
	uaddr = flag.String("uplink", ":4000", "uplink callback listening address:port")
	haddr = flag.String("http", "", "debug/metrics http server listening address:port")
)

func handleUplink(ctx context.Context, conn *grpc.ClientConn) error {
	defer conn.Close()

	up := uplinkpb.NewUplinkClient(conn)

	req, err := up.Register(ctx, &uplinkpb.RegisterTrigger{})
	if err != nil {
		return errors.Trace(err)
	}
	glog.Infof("got uplink request: %s", req)

	return nil
}

func listenUplink(uaddr string) {
	lis, err := net.Listen("tcp", uaddr)
	if err != nil {
		glog.Fatalf("could not listen: %v", err)
	}
	defer lis.Close()

	for {
		glog.Infof("waiting for uplinks")
		incoming, err := lis.Accept()
		if err != nil {
			glog.Fatalf("couldn't accept %v", err)
		}
		incomingConn, err := yamux.Client(incoming, yamux.DefaultConfig())
		if err != nil {
			glog.Fatalf("couldn't create yamux %v", err)
		}

		conn, err := grpc.Dial(":7777", grpc.WithInsecure(),
			grpc.WithDialer(func(target string, timeout time.Duration) (net.Conn, error) {
				return incomingConn.Open()
			}),
		)
		if err != nil {
			glog.Fatalf("did not connect: %s", err)
		}

		go func() {
			glog.Infof("Handling uplink from %q", incoming.RemoteAddr())

			if err := handleUplink(context.Background(), conn); err != nil {
				glog.Errorf("%+v", err)
			}
		}()
	}
}

func listenHttp(haddr string) error {
	if haddr == "" {
		select {}
	}
	mux := http.DefaultServeMux

	mux.Handle("/metrics", promhttp.Handler())
	clientIPWrapper, _ := forwarded.New("0.0.0.0/0", false, false, "X-Forwarded-For", "X-Forwarded-Protocol")

	return errors.Trace(http.ListenAndServe(haddr, clientIPWrapper.Handler(promhttpmux.Instrument(mux))))
}

func run(uaddr, haddr string) error {
	grpc.EnableTracing = true
	grpc_prometheus.EnableHandlingTimeHistogram()
	trace.AuthRequest = func(*http.Request) (bool, bool) { return true, true }

	go listenUplink(uaddr)

	return errors.Trace(listenHttp(haddr))
}

func main() {
	flag.Parse()
	defer glog.Flush()

	if err := run(*uaddr, *haddr); err != nil {
		glog.Fatalf("%+v", err)
	}
}
