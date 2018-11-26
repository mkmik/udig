package ingress

import (
	"crypto/tls"
	"fmt"
	"strconv"
	"strings"

	"github.com/bitnami-labs/udig/pkg/uplink"
	"github.com/golang/glog"
	"github.com/juju/errors"
)

var (
	DefaultPorts = []int32{443}
)

func ParsePorts(portStrings []string) ([]int32, error) {
	var res []int32
	for _, p := range portStrings {
		i, err := strconv.Atoi(p)
		if err != nil {
			return nil, errors.Errorf("port number %q is not a number: %v", p, err)
		}
		res = append(res, int32(i))
	}
	if len(res) == 0 {
		res = DefaultPorts
	}
	return res, nil
}

func Listen(port int32, cert tls.Certificate, forward chan<- uplink.NewStream) error {
	glog.Infof("listening ingress on %d", port)

	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}
	lis, err := tls.Listen("tcp", fmt.Sprintf(":%d", port), cfg)
	if err != nil {
		glog.Fatalf("%+v", err)
	}

	for {
		conn, err := lis.Accept()
		if err != nil {
			glog.Errorf("%+v", err)
			continue
		}
		t := conn.(*tls.Conn)

		// explicit hanshake is needed because we need to read the SNI value
		// out of the connection state before doing any read/write operation.
		if err := t.Handshake(); err != nil {
			glog.Errorf("%+v", err)
			continue
		}

		glog.Infof("accepted conn %p from %s for %s", conn, conn.RemoteAddr(), t.ConnectionState().ServerName)

		c := strings.SplitN(t.ConnectionState().ServerName, ".", 2)
		forward <- uplink.NewStream{TunnelID: c[0], Conn: conn}
	}

	return nil
}
