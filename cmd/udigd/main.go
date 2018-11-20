package main // imports "github.com/bitnami-labs/udig/cmd/udigd"

import (
	"flag"

	"github.com/golang/glog"
)

func run() error {
	return nil
}

func main() {
	flag.Parse()
	defer glog.Flush()

	if err := run(); err != nil {
		glog.Fatalf("%+v", err)
	}
}
