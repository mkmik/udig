package egress

import (
	"fmt"
	"io"
	"net"

	"github.com/golang/glog"
	"github.com/mkmik/udig/pkg/tunnel"
	"github.com/mkmik/udig/pkg/tunnel/tunnelpb"
)

// Server is an egress tunnel server.
type Server struct {
	tunnelpb.UnimplementedTunnelServer
	eaddr string
}

// NewServer creates an egress tunnel server.
func NewServer(eaddr string) (*Server, error) {
	return &Server{eaddr: eaddr}, nil
}

type closeWriter interface {
	CloseWrite() error
}

// NewStream implements the TunnelServer gRPC interface.
func (eg *Server) NewStream(s tunnelpb.Tunnel_NewStreamServer) error {
	cli, err := net.Dial("tcp", eg.eaddr)
	if err != nil {
		return err
	}
	defer cli.Close()

	done := make(chan error, 1)
	go func() (err error) {
		defer func() {
			glog.Infof("recv closed")
			cli.(closeWriter).CloseWrite()
			done <- err
		}()

		for {
			up, err := s.Recv()
			if err == io.EOF {
				return nil
			} else if err != nil {
				return err
			}
			glog.V(2).Infof("got: %v", up)
			fmt.Fprintf(cli, "%s", up.Data)
		}
	}()

	buf := make([]byte, tunnel.DefaultDataFrameSize)
	for {
		finish := false

		n, err := cli.Read(buf)
		if err == io.EOF {
			finish = true
		}

		s.Send(&tunnelpb.Down{
			Data:   buf[:n],
			Finish: finish,
		})
		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
	}

	return <-done
}
