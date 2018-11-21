package uplink

import (
	"context"

	"github.com/bitnami-labs/udig/pkg/uplink/uplinkpb"
)

type Server struct {
}

var _ uplinkpb.UplinkServer = (*Server)(nil)

func NewServer() (*Server, error) {
	return &Server{}, nil
}

func (s *Server) Ingress(server uplinkpb.Uplink_IngressServer) error {
	return nil
}

func (s *Server) Register(context.Context, *uplinkpb.RegisterTrigger) (*uplinkpb.RegisterRequest, error) {
	return nil, nil
}

func (s *Server) Setup(context.Context, *uplinkpb.SetupRequest) (*uplinkpb.SetupResponse, error) {
	return nil, nil
}
