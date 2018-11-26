package main

import (
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	// registers debug handlers
	_ "net/http/pprof"

	"github.com/bitnami-labs/promhttpmux"
	"github.com/bitnami-labs/udig/pkg/ingress"
	"github.com/bitnami-labs/udig/pkg/uplink"
	"github.com/bitnami-labs/udig/pkg/uplink/uplinkpb"
	"github.com/cockroachdb/cmux"
	"github.com/golang/glog"
	"github.com/grpc-ecosystem/go-grpc-middleware"
	"github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/hashicorp/yamux"
	"github.com/juju/errors"
	"github.com/mitchellh/go-homedir"
	"github.com/mmikulicic/stringlist"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	forwarded "github.com/stanvit/go-forwarded"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	laddr = flag.String("http", "", "listen address for http server (for debug, metrics)")
	taddr = flag.String("addr", "", "tunnel broker address")

	ingressPorts = stringlist.Flag("ingress-port", "requested ingress port(s); comma separated or repeated flag)")
	keyPairFile  = flag.String("keypair", filepath.Join(configDir, "keypair.json"), "Keypair file")
)

const (
	configDir = "~/.config/udiglink"
)

type KeyPair struct {
	Public  ed25519.PublicKey  `json:"public"`
	Private ed25519.PrivateKey `json:"private"`
}

func interceptors() []grpc.ServerOption {
	interceptors := []struct {
		stream grpc.StreamServerInterceptor
		unary  grpc.UnaryServerInterceptor
	}{
		{grpc_prometheus.StreamServerInterceptor, grpc_prometheus.UnaryServerInterceptor},
		{grpc_recovery.StreamServerInterceptor(), grpc_recovery.UnaryServerInterceptor()},
	}
	var (
		streamInterceptors []grpc.StreamServerInterceptor
		unaryInterceptors  []grpc.UnaryServerInterceptor
	)
	for _, i := range interceptors {
		streamInterceptors = append(streamInterceptors, i.stream)
		unaryInterceptors = append(unaryInterceptors, i.unary)
	}

	return []grpc.ServerOption{
		grpc.StreamInterceptor(grpc_middleware.ChainStreamServer(streamInterceptors...)),
		grpc.UnaryInterceptor(grpc_middleware.ChainUnaryServer(unaryInterceptors...)),
	}
}

func serve(up *uplink.Server, conn net.Listener) error {
	gs := grpc.NewServer(interceptors()...)

	reflection.Register(gs)
	uplinkpb.RegisterUplinkServer(gs, up)
	grpc_prometheus.Register(gs)

	glog.Infof("serving on %q", conn.Addr())
	return errors.Trace(gs.Serve(conn))
}

// keepDialing retries connecting when the connection fails
func keepDialing(up *uplink.Server, taddr string) {
	for {
		if err := dial(up, taddr); err != nil {
			glog.Errorf("%+v", err)
			time.Sleep(1 * time.Second)
		}
	}
}

// dial connects to a tunnel broker and sets up a grpc service listening
// in reverse through the client connection.
func dial(up *uplink.Server, taddr string) error {
	conn, err := net.DialTimeout("tcp", taddr, time.Second*5)
	if err != nil {
		return errors.Annotatef(err, "error dialing: %s", taddr)
	}

	grpcL, err := yamux.Server(conn, yamux.DefaultConfig())
	if err != nil {
		log.Fatalf("couldn't create yamux server: %s", err)
	}

	return errors.Trace(serve(up, grpcL))
}

// listen spawns a http server for debug (pprof, tracing, local debug uplink protocol)
func listen(up *uplink.Server, laddr string) error {
	if laddr == "" {
		select {}
	}

	mux := http.DefaultServeMux

	lis, err := net.Listen("tcp", laddr)
	if err != nil {
		return errors.Trace(err)
	}

	m := cmux.New(lis)

	grpcL := m.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
	httpL := m.Match(cmux.HTTP1Fast())

	mux.Handle("/metrics", promhttp.Handler())
	clientIPWrapper, _ := forwarded.New("0.0.0.0/0", false, false, "X-Forwarded-For", "X-Forwarded-Protocol")

	// Actually serve gRPC and HTTP
	go http.Serve(httpL, clientIPWrapper.Handler(promhttpmux.Instrument(mux)))
	go serve(up, grpcL)

	// Serve the multiplexer and block
	return errors.Trace(m.Serve())
}

func ensureKeypair(keyPairFile string) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	var keypair KeyPair
	f, err := os.Open(keyPairFile)
	if os.IsNotExist(err) {
		pub, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		keypair.Public = pub
		keypair.Private = priv

		if err := os.MkdirAll(filepath.Dir(keyPairFile), 0700); err != nil {
			return nil, nil, errors.Trace(err)
		}
		f, err := os.Create(keyPairFile)
		if err != nil {
			return nil, nil, errors.Trace(err)
		}
		defer f.Close()
		if err := json.NewEncoder(f).Encode(&keypair); err != nil {
			return nil, nil, errors.Trace(err)
		}
	} else if err != nil {
		return nil, nil, errors.Trace(err)
	} else {
		defer f.Close()
		if err := json.NewDecoder(f).Decode(&keypair); err != nil {
			return nil, nil, errors.Trace(err)
		}
	}
	return keypair.Public, keypair.Private, nil
}

func run(laddr, taddr string, ingressPorts []int32, keyPairFile string) error {
	grpc.EnableTracing = true
	grpc_prometheus.EnableHandlingTimeHistogram()
	trace.AuthRequest = func(*http.Request) (bool, bool) { return true, true }

	pub, priv, err := ensureKeypair(keyPairFile)
	if err != nil {
		return errors.Trace(err)
	}

	up, err := uplink.NewServer(ingressPorts, pub, priv)
	if err != nil {
		return errors.Trace(err)
	}

	go keepDialing(up, taddr)

	return errors.Trace(listen(up, laddr))
}

func main() {
	flag.Parse()
	defer glog.Flush()

	if *taddr == "" {
		glog.Exitf("missing mandatory -addr")
	}

	ingressPortNums, err := ingress.ParsePorts(*ingressPorts)
	if err != nil {
		glog.Exitf("%v", err)
	}

	keyPairFile, err := homedir.Expand(*keyPairFile)
	if err != nil {
		glog.Fatalf("%+v", err)
	}

	if err := run(*laddr, *taddr, ingressPortNums, keyPairFile); err != nil {
		glog.Fatalf("%+v", err)
	}
}
