package uplink

import (
	"context"

	"github.com/bitnami-labs/udig/pkg/uplink/uplinkpb"

	"github.com/golang/glog"
	"golang.org/x/crypto/ed25519"
)

type Server struct {
	privateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	Ports      []int32
	sup        chan<- StatusUpdate
}

var _ uplinkpb.UplinkServer = (*Server)(nil)

type StatusUpdate struct {
	Ingress []string
}

func NewServer(ingressPorts []int32, pub ed25519.PublicKey, priv ed25519.PrivateKey, sup chan<- StatusUpdate) (*Server, error) {
	return &Server{
		privateKey: priv,
		PublicKey:  pub,
		Ports:      ingressPorts,
		sup:        sup,
	}, nil
}

func (s *Server) Register(ctx context.Context, req *uplinkpb.RegisterTrigger) (*uplinkpb.RegisterRequest, error) {
	sig := ed25519.Sign(s.privateKey, req.Nonce)
	return &uplinkpb.RegisterRequest{
		Ed25519PublicKey: s.PublicKey,
		Signature:        sig,
		Ports:            s.Ports,
	}, nil
}

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
