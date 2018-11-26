package egress

import (
	"fmt"
	"io"

	"github.com/bitnami-labs/udig/pkg/tunnel/tunnelpb"
	"github.com/golang/glog"
)

type Server struct {
	eaddr string
}

func NewServer(eaddr string) (*Server, error) {
	return &Server{eaddr: eaddr}, nil
}

var _ tunnelpb.TunnelServer = (*Server)(nil)

func (eg *Server) NewStream(s tunnelpb.Tunnel_NewStreamServer) error {
	// TODO(mkm): establish egress connection and do two way siphoning
	for {
		up, err := s.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			glog.Errorf("%T: %v", err, err)
			return err
		}
		glog.V(2).Infof("got: %v", up)
		fmt.Printf("%s", up.Data)
	}
	glog.Infof("new stream done")
	return nil
}
