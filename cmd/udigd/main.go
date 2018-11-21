package main // imports "github.com/bitnami-labs/udig/cmd/udigd"

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
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
	cid "github.com/ipfs/go-cid"
	"github.com/juju/errors"
	multibase "github.com/multiformats/go-multibase"
	"github.com/multiformats/go-multihash"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	forwarded "github.com/stanvit/go-forwarded"
	"golang.org/x/crypto/ed25519"
	"golang.org/x/net/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

const (
	// this should live in multihash
	Ed25519Pub = 0xed
)

var (
	uaddr  = flag.String("uplink", ":4000", "uplink callback listening address:port")
	haddr  = flag.String("http", "", "debug/metrics http server listening address:port")
	domain = flag.String("domain", "udig.io", "domain name used for ingress adresses")
)

func handleUplink(ctx context.Context, conn *grpc.ClientConn, domain string) (err error) {
	defer conn.Close()

	up := uplinkpb.NewUplinkClient(conn)

	nonce := make([]byte, 64)
	if _, err := rand.Read(nonce[:]); err != nil {
		return errors.Trace(err)
	}

	req, err := up.Register(ctx, &uplinkpb.RegisterTrigger{
		Nonce: nonce,
	})
	if err != nil {
		return errors.Trace(err)
	}
	glog.V(2).Infof("got uplink request: %s", req)

	// From now on, any error we generate will be relayed to the uplink target via a Setup message.
	defer func() {
		if err != nil {
			st, ok := status.FromError(err)
			if !ok {
				glog.Errorf("cannot construct grpc status from: %+v", err)
			}
			if _, err := up.Setup(ctx, &uplinkpb.SetupRequest{
				Setup: &uplinkpb.SetupRequest_Error{Error: st.Proto()},
			}); err != nil {
				glog.Errorf("cannot send back Register errors via Setup: %+v", err)
			}
			return
		}
	}()

	if ok := ed25519.Verify(ed25519.PublicKey(req.Ed25519PublicKey), nonce, req.Signature); !ok {
		return errors.Errorf("bad signature")
	}
	glog.V(2).Infof("signature ok")

	tid, err := mkTunnelID(req.Ed25519PublicKey)
	if err != nil {
		return errors.Trace(err)
	}
	glog.Infof("setting up uplink for tunnel %s", tid)

	var ins []string
	for _, port := range req.Ports {
		ins = append(ins, fmt.Sprintf("%s.%s:%d", tid, domain, port))
	}

	_, err = up.Setup(ctx, &uplinkpb.SetupRequest{
		Setup: &uplinkpb.SetupRequest_Ingress_{
			Ingress: &uplinkpb.SetupRequest_Ingress{
				Ingress: ins,
			},
		},
	})
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

func mkTunnelID(publicKey []byte) (string, error) {
	mh, err := multihash.Sum(publicKey, multihash.SHA2_256, -1)
	if err != nil {
		return "", errors.Trace(err)
	}
	c := cid.NewCidV1(Ed25519Pub, mh)
	return c.Encode(multibase.MustNewEncoder(multibase.Base32)), nil
}

func listenUplink(uaddr, domain string) {
	lis, err := net.Listen("tcp", uaddr)
	if err != nil {
		glog.Fatalf("could not listen: %v", err)
	}
	defer lis.Close()

	glog.Infof("waiting for uplinks")
	for {
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

			if err := handleUplink(context.Background(), conn, domain); err != nil {
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

func run(uaddr, haddr, domain string) error {
	grpc.EnableTracing = true
	grpc_prometheus.EnableHandlingTimeHistogram()
	trace.AuthRequest = func(*http.Request) (bool, bool) { return true, true }

	go listenUplink(uaddr, domain)

	return errors.Trace(listenHttp(haddr))
}

func main() {
	flag.Parse()
	defer glog.Flush()

	if err := run(*uaddr, *haddr, *domain); err != nil {
		glog.Fatalf("%+v", err)
	}
}
