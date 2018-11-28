package main // imports "github.com/bitnami-labs/udig/cmd/udigd"

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"flag"
	"fmt"
	"net"
	"net/http"
	"time"

	// registers debug handlers
	_ "net/http/pprof"

	"github.com/bitnami-labs/promhttpmux"
	"github.com/bitnami-labs/udig/pkg/ingress"
	"github.com/bitnami-labs/udig/pkg/tunnel/tunnelpb"
	"github.com/bitnami-labs/udig/pkg/uplink"
	"github.com/bitnami-labs/udig/pkg/uplink/uplinkpb"
	"github.com/golang/glog"
	"github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/hashicorp/yamux"
	cid "github.com/ipfs/go-cid"
	"github.com/juju/errors"
	"github.com/mmikulicic/stringlist"
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
	// Ed25519Pub should live in multihash
	Ed25519Pub = 0xed
)

var (
	uaddr  = flag.String("uplink", ":4000", "uplink callback listening address:port")
	haddr  = flag.String("http", "", "debug/metrics http server listening address:port")
	domain = flag.String("domain", "udig.io", "domain name used for ingress adresses")

	ports = stringlist.Flag("port", "enabled ingress port(s); comma separated or repeated flag)")

	certPath = flag.String("cert", "", "path to PEM encoded x509 certificate for ingress server")
	keyPath  = flag.String("key", "", "path to PEM encoded private key for ingress server")
)

func handleUplink(ctx context.Context, conn *grpc.ClientConn, domain string, enabledPorts []int32, changeUplink chan<- uplink.Change) (err error) {
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
	for _, port := range effectivePorts(req.Ports, enabledPorts) {
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

	changeUplink <- uplink.Change{
		TunnelID: tid,
		UplinkID: conn.Target(),
		Client:   tunnelpb.NewTunnelClient(conn),
	}

	<-ctx.Done()

	changeUplink <- uplink.Change{
		TunnelID: tid,
		UplinkID: conn.Target(),
		Client:   nil,
	}

	return nil
}

func effectivePorts(requestedPorts, enabledPorts []int32) []int32 {
	rpm := map[int32]bool{}
	for _, port := range requestedPorts {
		rpm[port] = true
	}

	var res []int32
	for _, port := range enabledPorts {
		if len(rpm) == 0 || rpm[port] {
			res = append(res, port)
		}
	}
	return res
}

func mkTunnelID(publicKey []byte) (string, error) {
	mh, err := multihash.Sum(publicKey, multihash.SHA2_256, -1)
	if err != nil {
		return "", errors.Trace(err)
	}
	c := cid.NewCidV1(Ed25519Pub, mh)
	return c.Encode(multibase.MustNewEncoder(multibase.Base32)), nil
}

func randomUplinkID() (string, error) {
	id := make([]byte, 32)
	_, err := rand.Read(id)
	if err != nil {
		return "", errors.Trace(err)
	}
	mh, err := multihash.Sum(id, multihash.SHA2_256, -1)
	if err != nil {
		return "", errors.Trace(err)
	}
	c := cid.NewCidV1(cid.Raw, mh)
	return c.Encode(multibase.MustNewEncoder(multibase.Base32)), nil
}

func listenUplink(uaddr, domain string, enabledPorts []int32, changeUplink chan<- uplink.Change) {
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

		uplinkID, err := randomUplinkID()
		if err != nil {
			glog.Fatalf("%+v", err)
		}

		conn, err := grpc.Dial(uplinkID, grpc.WithInsecure(),
			grpc.WithDialer(func(target string, timeout time.Duration) (net.Conn, error) {
				return incomingConn.Open()
			}),
		)
		if err != nil {
			glog.Fatalf("did not connect: %s", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-incomingConn.CloseChan()
			cancel()
		}()

		// handleUplink now doesn't have to be aware of the underlying transport contortions
		// and can work with a high level grpc connection. When the underlying connection goes away
		// the context will be canceled.

		go func() {
			glog.Infof("Handling uplink from %q", incoming.RemoteAddr())

			if err := handleUplink(ctx, conn, domain, enabledPorts, changeUplink); err != nil {
				glog.Errorf("%+v", err)
			}
		}()
	}
}

func listenHTTP(haddr string) error {
	if haddr == "" {
		select {}
	}
	mux := http.DefaultServeMux

	mux.Handle("/metrics", promhttp.Handler())
	clientIPWrapper, _ := forwarded.New("0.0.0.0/0", false, false, "X-Forwarded-For", "X-Forwarded-Protocol")

	return errors.Trace(http.ListenAndServe(haddr, clientIPWrapper.Handler(promhttpmux.Instrument(mux))))
}

func run(uaddr, haddr, domain string, ports []int32, certPath, keyPath string) error {
	grpc.EnableTracing = true
	grpc_prometheus.EnableHandlingTimeHistogram()
	trace.AuthRequest = func(*http.Request) (bool, bool) { return true, true }

	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return errors.Trace(err)
	}

	mux := uplink.NewInProcessRouter()

	go listenUplink(uaddr, domain, ports, mux.Uplink())
	for _, p := range ports {
		go ingress.Listen(p, cert, mux.Ingress())
	}

	return errors.Trace(listenHTTP(haddr))
}

func main() {
	flag.Parse()
	defer glog.Flush()

	enabledPorts, err := ingress.ParsePorts(*ports)
	if err != nil {
		glog.Exitf("%v", err)
	}

	if *certPath == "" || *keyPath == "" {
		glog.Exitf("-cert and -key are manadatory")
	}

	if err := run(*uaddr, *haddr, *domain, enabledPorts, *certPath, *keyPath); err != nil {
		glog.Fatalf("%+v", err)
	}
}
