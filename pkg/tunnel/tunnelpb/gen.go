//go:generate protoc -I. -I../../.. --go_out=../../.. --go_opt=paths=source_relative --go-grpc_out=../../.. --go-grpc_opt=paths=source_relative pkg/tunnel/tunnelpb/tunnel.proto

package tunnelpb
