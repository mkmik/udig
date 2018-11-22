package ingress

import (
	"strconv"

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