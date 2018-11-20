//go:generate protoc -I. -I../../.. --go_out=paths=source_relative:../../.. pkg/uplink/uplinkpb/uplink.proto

package uplinkpb