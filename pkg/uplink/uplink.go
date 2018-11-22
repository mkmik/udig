package uplink

import (
	"context"

	"github.com/bitnami-labs/udig/pkg/uplink/uplinkpb"

	"github.com/golang/glog"
	"github.com/juju/errors"
	"golang.org/x/crypto/ed25519"
)

type Server struct {
	privateKey ed25519.PrivateKey
	PublicKey  ed25519.PublicKey
	Ports      []int32
}

var _ uplinkpb.UplinkServer = (*Server)(nil)

func NewServer(ingressPorts []int32) (*Server, error) {
	pub, priv, err := ed25519.GenerateKey(nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &Server{
		privateKey: priv,
		PublicKey:  pub,
		Ports:      ingressPorts,
	}, nil
}

func (s *Server) Ingress(server uplinkpb.Uplink_IngressServer) error {
	return nil
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
		return &uplinkpb.SetupResponse{}, nil
	} else {
		return &uplinkpb.SetupResponse{}, nil
	}
}
