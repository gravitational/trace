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




