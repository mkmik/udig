//go:generate go get -d github.com/googleapis/googleapis@v0.0.0-20181120215828-b7a1d68ea384
//go:generate bash -c "protoc -I. -I../../.. --go_out=../../.. --go_opt=paths=source_relative --go-grpc_out=../../.. --go-grpc_opt=paths=source_relative -I $GOPATH/pkg/mod/github.com/googleapis/googleapis@v0.0.0-20181120215828-b7a1d68ea384 pkg/uplink/uplinkpb/uplink.proto"

package uplinkpb
