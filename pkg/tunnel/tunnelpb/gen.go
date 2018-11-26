//go:generate protoc -I. -I../../.. --go_out=plugins=grpc,paths=source_relative:../../.. pkg/tunnel/tunnelpb/tunnel.proto

package tunnelpb
