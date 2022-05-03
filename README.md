# udig

[![Build Status](https://img.shields.io/github/workflow/status/mkmik/udig/CI)](https://github.com/mkmik/udig/actions)
[![Go Report Card](https://goreportcard.com/badge/github.com/mkmik/udig)](https://goreportcard.com/report/github.com/mkmik/udig)

udig is a public-key addressed TCP tunnel software. It allows anybody to expose a local network
service through a public stable ingress, even if the local service is behind a NAT or firewall.


> This project is still under heavy work in progress

## Background

There are things like ngrok and other commercial software.
udig is suitable for automation because users don't need to create any account.

## How it works

![architecture](doc/arch.png)

It has of two logical endpoints:

* **uplink**: clients building a tunnel open a TCP connection to our servers and sets up a gRPC service listening on the client connection (yep in reverse; nothing new).
* **ingress**: another endpoint is for connections entering the tunnel; traffic is then forwarded to any active uplink active matching a tunnel ID encoded in the TLS SNI field.


Tunnel IDs are base32 encoded hashes of the public key (technically a [multihash](https://github.com/multiformats/multihash) encoded as a base32 [multibase](https://github.com/multiformats/multibase)).

Tunnel ingress addresses look like this: bahwqcerazdp76ea6rpuwvbbwxkjtypdntmw4bohi6amkzkfz2kswpxlpgykq.udig.io

## Install

```
$ go get -u github.com/mkmik/udig/...
```

## Client usage

Shell 1:
```
$ udiglink -R 443:localhost:8080
https://bahwqcerazdp76ea6rpuwvbbwxkjtypdntmw4bohi6amkzkfz2kswpxlpgykq
```

Shell 2:
```
$ python3 -m http.server 8080
Serving HTTP on 0.0.0.0 port 8080 (http://0.0.0.0:8080/) ...
```

Shell 3:
```
$ curl https://bahwqcerazdp76ea6rpuwvbbwxkjtypdntmw4bohi6amkzkfz2kswpxlpgykq.udig.io/README.md
# udig
...
```

## Run locally

Shell 1:
```
$ (cd cmd/udiglink && go build && ./udiglink -alsologtostderr -addr localhost:4000 -http :8081 -R 8443:localhost:1234)
```

Shell 2:
```
$ (cd cmd/udigd && go build && ./udigd -alsologtostderr -http :8001 -port 8080 -port 8443 -cert ../../pkg/ingress/testdata/cert.pem -key ../../pkg/ingress/testdata/key.pem)
```

Shell 3:
```
$ python3 -m http.server 1234
```

Shell 4:
```
$ curl --connect-to ::127.0.0.1:8443 -k https://bahwqcerazdp76ea6rpuwvbbwxkjtypdntmw4bohi6amkzkfz2kswpxlpgykq.udig.io/README.md
```

(use the actual hostname you get in `tunnel ingress addresses: ["bahw....` in Shell 1 for ^^^)

> NOTE: This command requires `curl` version 7.49.0 or above.

For earlier versions of `curl` this can be tested by adding the host to `/etc/hosts` file as `127.0.0.1` (or `C:\Windows\System32\drivers\etc\hosts` on Windows) - like:
```
127.0.0.1 bahwqcerazdp76ea6rpuwvbbwxkjtypdntmw4bohi6amkzkfz2kswpxlpgykq.udig.io
```

And then running the curl command with port 8443:

```
curl https://bahwqcerazdp76ea6rpuwvbbwxkjtypdntmw4bohi6amkzkfz2kswpxlpgykq.udig.io:8443/README.md
```

## Off-the-shelf tunnel client example

Udig forces you to use a TLS client and one that supports SNI nonetheless!
If you have a plaintext TCP client on one side that needs to talk to a plaintext TCP  server on the other side of the tunnel, this is an example of how you can setup the client side of the tunnel with standard off the-shelf-tools:

```
$ echo "openssl s_client -quiet -connect $DST:$PORT -servername $DST" >/tmp/cmd.sh
$ chmod +x /tmp/cmd.sh
$ socat TCP-LISTEN:1234,reuseaddr,fork 'SYSTEM:/tmp/cmd.sh' &
```


## Contributing

PRs accepted
