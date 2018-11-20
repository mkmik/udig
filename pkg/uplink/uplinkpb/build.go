//go:generate protoc -I. -I../../.. --go_out=plugins=grpc,paths=source_relative:../../.. pkg/uplink/uplinkpb/uplink.proto

package uplinkpb