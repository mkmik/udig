package uplink

import (
	"context"
	"net"

	"github.com/bitnami-labs/udig/pkg/tunnel"
	"github.com/bitnami-labs/udig/pkg/tunnel/tunnelpb"
	"github.com/golang/glog"
)

// A NewStream struct encapsulates an intent to tunnel a new connection
// for a given tunnel ID on any uplink that can fullfull that request.
type NewStream struct {
	TunnelID string
	Conn     net.Conn
}

// A Router receives NewStream requests and forwards them to the appropriate uplink.
// If no uplink is found the connection contianed in the NewStream request will be
// terminated.
type Router interface {
	Ingress() chan<- NewStream
}

type UplinkChange struct {
	TunnelID string
	UplinkID string                // something unique about the uplink connection
	Client   tunnelpb.TunnelClient // if nil, uplink instance removed
}

// InProcessRouter connects uplinks and ingresses in the same process.
type InProcessRouter struct {
	ingress chan NewStream
	uplink  chan UplinkChange
	m       map[string]map[string]tunnelpb.TunnelClient
}

func NewInProcessRouter() *InProcessRouter {
	r := &InProcessRouter{
		ingress: make(chan NewStream),
		uplink:  make(chan UplinkChange),
		m:       map[string]map[string]tunnelpb.TunnelClient{},
	}

	go r.run()
	return r
}

func (r *InProcessRouter) Ingress() chan<- NewStream   { return r.ingress }
func (r *InProcessRouter) Uplink() chan<- UplinkChange { return r.uplink }

func (r *InProcessRouter) run() {
	for {
		select {
		case up := <-r.uplink:
			glog.Infof("got uplink change request: %v", up)
			if _, ok := r.m[up.TunnelID]; !ok {
				r.m[up.TunnelID] = map[string]tunnelpb.TunnelClient{}
			}
			ups := r.m[up.TunnelID]
			if up.Client != nil {
				ups[up.UplinkID] = up.Client
			} else {
				delete(ups, up.UplinkID)
			}
			if len(ups) == 0 {
				delete(r.m, up.TunnelID)
			}

			glog.Infof("now uplink map is: %v", r.m)

		case in := <-r.ingress:
			glog.Infof("got new stream request: %v", in)
			// poor man's round robin based on the pseudo randomization that Go runtime provides
			// to map key iteration order.
			// TODO(mkm) use real round-robin
			found := false
			for _, up := range r.m[in.TunnelID] {
				found = true
				glog.Infof("TODO using %v", up)
				hdr := tunnel.HeaderFor(in.TunnelID, in.Conn)
				tunnel.Siphon(context.Background(), up, hdr, in.Conn)
				break
			}

			if !found {
				glog.Errorf("cannot find any uplink for tunnel %q", in.TunnelID)
				in.Conn.Close()
			}
		}
	}
}
