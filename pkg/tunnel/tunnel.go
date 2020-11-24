package tunnel

import (
	"context"
	"fmt"
	"io"
	"net"

	"github.com/bitnami-labs/udig/pkg/tunnel/tunnelpb"
	"github.com/golang/glog"
)

const (
	// DefaultDataFrameSize is the default data frame size.
	DefaultDataFrameSize = 1024 * 16
)

// HeaderFor returns a header for a given tunnel ID and connection.
func HeaderFor(tunnelID string, conn net.Conn) *tunnelpb.Up_Header {
	return &tunnelpb.Up_Header{
		TunnelId: tunnelID,
		// TODO(mkm): other stuff
	}
}

// Siphon connects a network connection with a Tunnel gRPC service
// and copies data bidirectionally.
// If the header is not nil it will be sent in the first up frame.
func Siphon(ctx context.Context, tunnel tunnelpb.TunnelClient, header *tunnelpb.Up_Header, conn net.Conn) error {
	s, err := tunnel.NewStream(ctx)
	if err != nil {
		return fmt.Errorf("error siphoning: %w", err)
	}

	go func() {
		data := make([]byte, DefaultDataFrameSize)

		for {
			finish := false

			n, err := conn.Read(data)
			if err == io.EOF {
				finish = true
			}

			glog.Infof("sending %d bytes up, eof? %v", n, err == io.EOF)
			s.Send(&tunnelpb.Up{
				Header: header,
				Data:   data[:n],
				Finish: finish, // TODO(mkm): figure out if we really need this in this direction.
			})
			if err == io.EOF {
				s.CloseSend()
				break
			}
			header = nil
		}

		glog.Infof("done with up siphoning")
	}()

	go func() {
		for {
			down, err := s.Recv()
			if err == io.EOF {
				break
			} else if err != nil {
				glog.Errorf("got down error: %v", err)
				break
			}
			glog.Infof("receiving %d bytes down", len(down.Data))
			glog.V(2).Infof("got down: %v", down)
			conn.Write(down.Data)
		}
		glog.Infof("done with down siphoning")
	}()

	return nil
}
