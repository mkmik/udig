package ingress

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"

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

func Listen(port int32) (err error) {
	glog.Infof("listening ingress on %d", port)

	cfg := &tls.Config{
		GetCertificate: func(info *tls.ClientHelloInfo) (*tls.Certificate, error) {
			glog.Infof("Got hello with SNI %q", info.ServerName)
			return nil, errors.Errorf("not implemented")
		},
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
		go handle(conn)
	}

	return nil
}

func handle(conn net.Conn) {
	glog.Infof("accepted conn %v", conn)
	io.Copy(os.Stdout, conn)
}
