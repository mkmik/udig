// Package forwarded offers a decorator for http.Handler that parses
// Forwarded header (RFC7239) or individual X-Forwarded-For and X-Forarded-Protocol-alike
// headers and updates http.Request with the detected IP address and protocol.
// The headers are accepted from the list of trusted IP addresses/networks only.
//
// When IP address is parsed from the configured header, the request.RemoteAddr is updated
// with the addess and fake port "65535", since http.Request defines that the port has to be present.
//
// When https is detected, but the request doesn't contain TLS information, an empty tls.ConnectionState
// is attached to the http.Request. Obviously, it doesn't contain any information about
// encryption and certificates, but could serve as an indicator that some encryption is astually in place.
//
// When http is detected, Request.TLS is reset to nil to indicate that no encryption was used.
//
// In addition, IPNets ipmlements a slice of net.IPNet values with the ability
// to parse comma-delimited IPv4 and IPv6 addresses and CIDR networks
// (optionally using flag package) and then check if individual net.IP is matching any of
// these networks
package forwarded

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/textproto"
	"strings"
)

// parseForwarded parses the "Forwarded" header and returns
// IP address and protocol fields as strings. If any field is not present,
// empty string is returned
func parseForwarded(forwarded string) (addr, proto string) {
	for _, forwardedPair := range strings.Split(forwarded, ";") {
		if tv := strings.SplitN(forwardedPair, "=", 2); len(tv) == 2 {
			token, value := tv[0], tv[1]
			token = strings.TrimSpace(token)
			value = strings.TrimSpace(strings.Trim(value, `"`))
			switch strings.ToLower(token) {
			case "for":
				addr = value
			case "proto":
				proto = value
			}
		}
	}
	return addr, proto
}

// lastestHeader gets the latest value of any given header: the rightmost value after
// the last ", " from the lowerest header in the request.
func latestHeader(r *http.Request, h string) (val string) {
	if values, ok := r.Header[h]; ok {
		latestHeaderInstance := values[len(values)-1]
		instanceValues := strings.Split(latestHeaderInstance, ", ")
		return instanceValues[len(instanceValues)-1]
	}
	return ""
}

// getIP parses r.RemoteAddr from *http.Request. If the value is "@", which means
// that a unix domain socket is in use, it returns nil, otherwise it tries to parse
// ip:port pair and returns net.IP on success, or nil and an error otherwise.
func getIP(r *http.Request) (ip net.IP, err error) {
	ipString := r.RemoteAddr
	if ipString == "@" {
		return nil, nil
	}
	if ipNoport, _, err := net.SplitHostPort(ipString); err == nil {
		ipString = ipNoport
	}
	ip = net.ParseIP(ipString)
	if ip == nil {
		return nil, fmt.Errorf("failed to parse IP %v", ipString)
	}
	return ip, nil
}

// Wrapper is a configuration structure for the Handler wrapper
type Wrapper struct {
	AllowedNets    IPNets // A slice of networks that are allowed to set the *Forwarded* headers
	AllowEmptySrc  bool   // Trust empty remote address (for example, Unix Domain Sockets)
	ParseForwarded bool   // Parse Forwarded (rfc7239) header. If set to true, other headers are ignored
	ForHeader      string // A header with the actual IP address[es] (For example, "X-Forwarded-For")
	ProtocolHeader string // A header with the protocol name (http or https. For example "X-Forwarded-Protocol")
}

// New parses comma-separated list of IP addresses and/or CIDR subnets and returns configured *Wrapper
func New(nets string, allowEmpty, parseForwarded bool, forHeader, protocolHeader string) (wrapper *Wrapper, err error) {
	wrapper = &Wrapper{AllowEmptySrc: allowEmpty, ParseForwarded: parseForwarded, ForHeader: forHeader, ProtocolHeader: protocolHeader}
	// normalise X-Forwarded-* headers (`sonme-header` -> `Some-Header`)
	wrapper.ForHeader = textproto.CanonicalMIMEHeaderKey(wrapper.ForHeader)
	wrapper.ProtocolHeader = textproto.CanonicalMIMEHeaderKey(wrapper.ProtocolHeader)
	// parse comma-separated list of IPs and CIDRs and store it at wrapper.AllowedNets
	if err := wrapper.AllowedNets.Set(nets); err != nil {
		return nil, err
	}
	return wrapper, nil
}

// update parses the *http.Reuqest and updates it with IP address and protocol from the headers
// if they are present and parse without error
func (wrapper *Wrapper) update(r *http.Request) {
	var addr, proto string
	if wrapper.ParseForwarded {
		// parse Forwarded:, ignore other headers
		if forwarded := latestHeader(r, "Forwarded"); forwarded != "" {
			addr, proto = parseForwarded(forwarded)
		}
	} else {
		if wrapper.ForHeader != "" {
			addr = strings.TrimSpace(strings.Trim(latestHeader(r, wrapper.ForHeader), `"`))
		}
		if wrapper.ProtocolHeader != "" {
			proto = strings.TrimSpace(latestHeader(r, wrapper.ProtocolHeader))
		}
	}
	if addr != "" {
		if _, _, err := net.SplitHostPort(addr); err != nil {
			// If the IP isn't parsebale, we just replace it with 0.0.0.0
			if net.ParseIP(addr) == nil {
				addr = "0.0.0.0"
			}
			// well, it's fake, but we need some port here
			addr = net.JoinHostPort(addr, "65535")
		}
		r.RemoteAddr = addr
	}
	if strings.ToLower(proto) == "https" && r.TLS == nil {
		// apparently, its the only way to indicate that https is in use
		r.TLS = new(tls.ConnectionState)
	}
	if strings.ToLower(proto) == "http" {
		r.TLS = nil
	}
}

// Handler offers decorator for a http.Handler. It analyses incoming requests and, if source IP
// matches the trusted IP/nets list, updates the request with IP address and encryption information.
func (wrapper *Wrapper) Handler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip, err := getIP(r); err == nil && ((ip == nil && wrapper.AllowEmptySrc) || (ip != nil && wrapper.AllowedNets.Contains(ip))) {
			wrapper.update(r)
		}
		h.ServeHTTP(w, r)
	})
}
