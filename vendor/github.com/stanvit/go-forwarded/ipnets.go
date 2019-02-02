package forwarded

import (
	"net"
	"strings"
)

// IPNets is a slice of net.IPNet
type IPNets []net.IPNet

// String returns a string with comma-delimited CIDR representations of
// networks stored in IPNets
// Together with Set implements flag.Value interface so it's possible to parse command-line parameters directly into *IPnets
func (nets *IPNets) String() string {
	s := make([]string, len(*nets))
	for i, net := range *nets {
		s[i] = net.String()
	}
	return strings.Join(s, ", ")
}

// Set parses supplied comma-delimited list of IPv4 or IPv6 IPs and CIDR networks into IPNets.
// Together with String implements flag.Value interface so it's possible to parse command-line parameters directly into *IPnets
func (nets *IPNets) Set(param string) error {
	var ipnet *net.IPNet
	var err error
	for _, s := range strings.Split(param, ",") {
		s = strings.TrimSpace(s)
		// no "/" symbol in the address, not CIDR
		if !strings.Contains(s, "/") {
			if strings.Contains(s, ".") {
				s = s + "/32"
			} else if strings.Contains(s, ":") {
				s = s + "/128"
			}
		}
		if _, ipnet, err = net.ParseCIDR(s); err != nil {
			return err
		}
		*nets = append(*nets, *ipnet)
	}
	return nil
}

// Contains returns true if ip matches any of networks stored in IPNets
func (nets IPNets) Contains(ip net.IP) bool {
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
