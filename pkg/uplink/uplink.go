package uplink

import (
	"context"

	"github.com/golang/glog"
	"github.com/mkmik/udig/pkg/uplink/uplinkpb"
	"golang.org/x/crypto/ed25519"
)

// Server is an uplink server.
type Server struct {
	uplinkpb.UnimplementedUplinkServer
	privateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	Ports      []int32
	sup        chan<- StatusUpdate
}

// StatusUpdate is used to report
type StatusUpdate struct {
	Ingress []string
}

// NewServer creates a new uplink mapped on a list of ingress ports.
//
// The tunnel server can asynchronously advertise ingress addresses, which will be sent as StatusUpdate
// structures in the sup channel (possibly multiple times).
func NewServer(ingressPorts []int32, pub ed25519.PublicKey, priv ed25519.PrivateKey, sup chan<- StatusUpdate) (*Server, error) {
	return &Server{
		privateKey: priv,
		PublicKey:  pub,
		Ports:      ingressPorts,
		sup:        sup,
	}, nil
}

// Register implements the uplink gRPC service.
func (s *Server) Register(ctx context.Context, req *uplinkpb.RegisterTrigger) (*uplinkpb.RegisterRequest, error) {
	sig := ed25519.Sign(s.privateKey, req.Nonce)
	return &uplinkpb.RegisterRequest{
		Ed25519PublicKey: s.PublicKey,
		Signature:        sig,
		Ports:            s.Ports,
	}, nil
}

// Setup implements the uplink gRPC service.
func (s *Server) Setup(cxt context.Context, req *uplinkpb.SetupRequest) (*uplinkpb.SetupResponse, error) {
	if e := req.GetError(); e != nil {
		glog.Errorf("registration error: %s", e)
		return &uplinkpb.SetupResponse{}, nil
	} else if in := req.GetIngress(); in != nil {
		glog.Infof("tunnel ingress addresses: %q", in.Ingress)
		if s.sup != nil {
			s.sup <- StatusUpdate{Ingress: in.Ingress}
		}
		return &uplinkpb.SetupResponse{}, nil
	} else {
		return &uplinkpb.SetupResponse{}, nil
	}
}
