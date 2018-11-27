# udig

udig is a public-key addressed TCP tunnel software. It allows anybody to expose a local network
service through a public stable ingress, even if the local service is behind a NAT or firewall.


> This project is still under heavy work in progress

## Background

There are things like ngrok and other commercial software.
udig is suitable for automation because users don't need to create any account.

## Example usage

Shell 1:
```
(cd cmd/udiglink && go build && ./udiglink -alsologtostderr -addr localhost:4000 -http :8081 -ingress-port 8443 -egress localhost:1234)
```

Shell 2:
```
(cd cmd/udigd && go build && ./udigd -alsologtostderr -http :8001  -port 8080 -port 8443 -cert ../../pkg/ingress/testdata/cert.pem -key ../../pkg/ingress/testdata/key.pem)
```

Shell 3:
```
python3 -m http.server 1234
```

Shell 4:
```
curl  --connect-to ::127.0.0.1:8443 -k https://bahwqcerazdp76ea6rpuwvbbwxkjtypdntmw4bohi6amkzkfz2kswpxlpgykq.udig.io/README.md
```

(use the actual hostname you get in `tunnel ingress addresses: ["bahw....` in Shell 1 for ^^^)

## Contributing

PRs accepted
