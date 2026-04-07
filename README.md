# Trace

[![GoDoc](https://godoc.org/github.com/gravitational/trace?status.png)](https://godoc.org/github.com/gravitational/trace)
![Test workflow](https://github.com/gravitational/trace/actions/workflows/test.yaml/badge.svg?branch=master)

Package for error handling and error reporting

Read more here:

https://goteleport.com/blog/golang-error-handling/

### Capture file, line and function

```golang

import (
     "github.com/gravitational/trace"
)

func someFunc() error {
   return trace.Wrap(err)
}


func main() {
  err := someFunc()
  fmt.Println(err.Error()) // prints file, line and function
}
```

### Build tags

The functions `WriteError`, `ReadError` and `ErrorToCode` require including the standard library package `net/http`, which transitively depends on `crypto/tls`, `crypto/x509`, and a lot of the `crypto/...` package tree, and the `ConvertSystemError` function depends on `crypto/x509` to wrap the `x509.SystemRootsError` and `x509.UnknownAuthorityError` errors into a `trace.TrustError`.

As a size optimization for binaries that don't otherwise make use of `net/http` or `crypto/x509`, builds with the `gravitational_trace.nocrypto` build tag will exclude the `WriteError`, `ReadError` and `ErrorToCode` functions, and will not match against the `x509.SystemRootsError` and `x509.UnknownAuthorityError` errors in `ConvertSystemError`, which will get rid of the requirement for `net/http` or `crypto/...`.
