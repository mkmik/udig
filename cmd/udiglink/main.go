package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	// registers debug handlers
	_ "net/http/pprof"

	"github.com/bitnami-labs/promhttpmux"
	"github.com/mkmik/udig/pkg/egress"
	"github.com/mkmik/udig/pkg/tunnel/tunnelpb"
	"github.com/mkmik/udig/pkg/uplink"
	"github.com/mkmik/udig/pkg/uplink/uplinkpb"
	"github.com/cockroachdb/cmux"
	"github.com/golang/glog"
	grpc_middleware "github.com/grpc-ecosystem/go-grpc-middleware"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/recovery"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/hashicorp/yamux"
	"github.com/mkmik/stringlist"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	forwarded "github.com/stanvit/go-forwarded"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var (
	laddr = flag.String("http", "", "listen address for http server (for debug, metrics)")
	taddr = flag.String("addr", "uplink.udig.io:4000", "tunnel broker address")
	maps  = stringlist.Flag("R", "remote_port:local_host:local_port; comma separated or repeated flag")

	keyPairFile = flag.String("keypair", filepath.Join(defaultConfigDir, "keypair.json"), "Keypair file")

	defaultConfigDir = getDefaultConfigDir()
)

func getDefaultConfigDir() string {
	h, err := os.UserHomeDir()
	if err != nil {
		panic(err)
	}
	return filepath.Join(h, ".config/udiglink")
}

// KeyPair is a ed25519 key pair JSON struct.
type KeyPair struct {
	Public  ed25519.PublicKey  `json:"public"`
	Private ed25519.PrivateKey `json:"private"`
}

// registerGRPC is a callback that is used to install gRPC services on a gRPC server.
type registerGRPC func(*grpc.Server)

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

func serve(reg registerGRPC, conn net.Listener) error {
	gs := grpc.NewServer(interceptors()...)

	reflection.Register(gs)
	grpc_prometheus.Register(gs)

	reg(gs)

	glog.Infof("serving gRPC on on %q", conn.Addr())
	return gs.Serve(conn)
}

// keepDialing retries connecting when the connection fails
func keepDialing(reg registerGRPC, taddr string) {
	for {
		if err := dial(reg, taddr); err != nil {
			glog.Errorf("%+v", err)
			time.Sleep(1 * time.Second)
		}
	}
}

// dial connects to a tunnel broker and sets up a grpc service listening
// in reverse through the client connection.
func dial(reg registerGRPC, taddr string) error {
	conn, err := net.DialTimeout("tcp", taddr, time.Second*5)
	if err != nil {
		return fmt.Errorf("error dialing %q: %w", taddr, err)
	}

	grpcL, err := yamux.Server(conn, yamux.DefaultConfig())
	if err != nil {
		log.Fatalf("couldn't create yamux server: %s", err)
	}

	if err := serve(reg, grpcL); err != nil {
		return fmt.Errorf("serve after dialing %q: %w", taddr, err)
	}
	return nil
}

// listen spawns a http server for debug (pprof, tracing, local debug uplink protocol)
func listen(reg registerGRPC, laddr string) error {
	if laddr == "" {
		select {}
	}

	mux := http.DefaultServeMux

	lis, err := net.Listen("tcp", laddr)
	if err != nil {
		return err
	}

	m := cmux.New(lis)

	grpcL := m.Match(cmux.HTTP2HeaderField("content-type", "application/grpc"))
	httpL := m.Match(cmux.HTTP1Fast())

	mux.Handle("/metrics", promhttp.Handler())
	clientIPWrapper, _ := forwarded.New("0.0.0.0/0", false, false, "X-Forwarded-For", "X-Forwarded-Protocol")

	// Actually serve gRPC and HTTP
	go http.Serve(httpL, clientIPWrapper.Handler(promhttpmux.Instrument(mux)))
	go serve(reg, grpcL)

	// Serve the multiplexer and block
	if err := m.Serve(); err != nil {
		return fmt.Errorf("serve after listening %q: %w", laddr, err)
	}
	return nil
}

func ensureKeypair(keyPairFile string) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	var keypair KeyPair
	f, err := os.Open(keyPairFile)
	if os.IsNotExist(err) {
		pub, priv, err := ed25519.GenerateKey(nil)
		if err != nil {
			return nil, nil, err
		}
		keypair.Public = pub
		keypair.Private = priv

		if err := os.MkdirAll(filepath.Dir(keyPairFile), 0700); err != nil {
			return nil, nil, err
		}
		f, err := os.Create(keyPairFile)
		if err != nil {
			return nil, nil, err
		}
		defer f.Close()
		if err := json.NewEncoder(f).Encode(&keypair); err != nil {
			return nil, nil, fmt.Errorf("encoding keypair %q: %w", keyPairFile, err)
		}
	} else if err != nil {
		return nil, nil, err
	} else {
		defer f.Close()
		if err := json.NewDecoder(f).Decode(&keypair); err != nil {
			return nil, nil, fmt.Errorf("decoding keypair %q: %w", keyPairFile, err)
		}
	}
	fmt.Fprintf(os.Stderr, "using key file: %s\n", keyPairFile)
	return keypair.Public, keypair.Private, nil
}

func run(laddr, taddr, eaddr string, ingressPorts []int32, keyPairFile string) error {
	grpc.EnableTracing = true
	grpc_prometheus.EnableHandlingTimeHistogram()
	trace.AuthRequest = func(*http.Request) (bool, bool) { return true, true }

	pub, priv, err := ensureKeypair(keyPairFile)
	if err != nil {
		return err
	}

	sup := make(chan uplink.StatusUpdate)
	go func() {
		for up := range sup {
			for _, i := range up.Ingress {
				fmt.Printf("%s\n", i)
			}
		}
	}()

	up, err := uplink.NewServer(ingressPorts, pub, priv, sup)
	if err != nil {
		return err
	}

	eg, err := egress.NewServer(eaddr)
	if err != nil {
		return err
	}

	reg := func(gs *grpc.Server) {
		uplinkpb.RegisterUplinkServer(gs, up)
		tunnelpb.RegisterTunnelServer(gs, eg)
	}

	go keepDialing(reg, taddr)

	return listen(reg, laddr)
}

// parses a slice of remote_port:local_host:local_port.
// for now we support only one egress
func parsePortMaps(portMaps []string) (ports []int32, egress string, err error) {
	for _, s := range portMaps {
		c := strings.SplitN(s, ":", 2)

		i, err := strconv.Atoi(c[0])
		if err != nil {
			return nil, "", fmt.Errorf("parsing port %q: %w", s, err)
		}

		if egress != "" && c[1] != egress {
			return nil, "", fmt.Errorf("we currently support only one egress, found %q and %q", egress, c[1])
		}
		egress = c[1]

		ports = append(ports, int32(i))
	}
	return ports, egress, err
}

func main() {
	flag.Parse()
	defer glog.Flush()

	if *taddr == "" {
		glog.Exitf("missing mandatory -addr")
	}

	if len(*maps) == 0 {
		glog.Exitf("requiring least one -R")
	}

	ingressPortNums, eaddr, err := parsePortMaps(*maps)
	if err != nil {
		glog.Exitf("%v", err)
	}

	if err := run(*laddr, *taddr, eaddr, ingressPortNums, *keyPairFile); err != nil {
		glog.Fatalf("%+v", err)
	}
}
