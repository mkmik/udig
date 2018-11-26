package uplink

import (
	"net"

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
			if ups, ok := r.m[in.TunnelID]; ok {
				glog.Infof("TODO: found uplink, forward to any of %v", ups)
			}
		}
	}
}
