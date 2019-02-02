# go-forwarded

[![Build Status](https://travis-ci.org/stanvit/go-forwarded.svg?branch=master)](https://travis-ci.org/stanvit/go-forwarded) [![GoDoc](https://godoc.org/github.com/stanvit/go-forwarded?status.svg)](https://godoc.org/github.com/stanvit/go-forwarded)


## Description

`forwarded` is a Golang decorator/wrapper for [http.Handler](https://golang.org/pkg/net/http/#Handler)
that parses `X-Forwarded-For` and `X-Forwarded-Protocol` headers and updates passing
[http.Request.RemoteAddr](https://golang.org/pkg/net/http/#Request) and
[http.Request.TLS](https://golang.org/pkg/net/http/#Request) accordingly.

It supports arbitrary named individual headers and [RFC7239](http://tools.ietf.org/html/rfc7239) `Forwarded` header.


## Usage example

Extremely simplified example:

```go
package main

import (
	"fmt"
	"github.com/stanvit/go-forwarded"
	"net/http"
)

func simpleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf("Requesting IP is %v\nHTTPS: %t\n", r.RemoteAddr, r.TLS != nil)))
}

func main() {
	wrapper, _ := forwarded.New("192.168.0.0/16, 127.0.0.1", false, false, "X-Forwarded-For", "X-Forwarded-Protocol")
	handler := wrapper.Handler(http.HandlerFunc(simpleHandler))
	http.Handle("/", handler)
	http.ListenAndServe(":8082", nil)
}
```

```
$ curl -H 'X-Forwarded-For: 1.2.3.4' -H 'X-Forwarded-Protocol: https' -v http://127.0.0.1:8082
* Rebuilt URL to: http://127.0.0.1:8082/
* Hostname was NOT found in DNS cache
*   Trying 127.0.0.1...
* Connected to 127.0.0.1 (127.0.0.1) port 8082 (#0)
> GET / HTTP/1.1
> User-Agent: curl/7.35.0
> Host: 127.0.0.1:8082
> Accept: */*
> X-Forwarded-For: 1.2.3.4
> X-Forwarded-Protocol: https
> 
< HTTP/1.1 200 OK
< Content-Type: text/plain
< Date: Sat, 05 Sep 2015 01:16:15 GMT
< Content-Length: 43
< 
Requesting IP is 1.2.3.4:65535
HTTPS: true
* Connection #0 to host 127.0.0.1 left intact
```


## API documentation

API documentation is avalable at [Godoc](https://godoc.org/github.com/stanvit/go-forwarded)
