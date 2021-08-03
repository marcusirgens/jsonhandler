# jsonhandler

[![Go Reference](https://pkg.go.dev/badge/github.com/marcusirgens/jsonhandler.svg)](https://pkg.go.dev/github.com/marcusirgens/jsonhandler)

The `jsonhandler` library creates JSON-speaking `http.Handler`s super fast.

## Installation

```bash
go get -u github.com/marcusirgens/jsonhandler
```

## Usage

Here's a webserver that greets users:

```go
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/marcusirgens/jsonhandler"
)

type Args struct {
	Name string `json:"name"`
}

type Result struct {
	Greeting string `json:"greeting"`
}

// JSONHandler responds to requests.
func JSONHandler(ctx context.Context, args Args) Result {
	return Result{
		Greeting: fmt.Sprintf("Hello, %s", args.Name),
	}
}

func main() {
	http.Handle("/greet", jsonhandler.NewHandler(JSONHandler))
	log.Fatal(http.ListenAndServe(":8080", nil))
}

```

Running this, we can make a call to verify that everything works as expected:

```shell
$ curl -ifd '{"name": "World"}' "http://localhost:8080/greet"                                                        

HTTP/1.1 200 OK
Content-Type: application/json; charset=utf-8
Date: Tue, 03 Aug 2021 19:25:27 GMT
Content-Length: 33

{
  "greeting": "Hello, World"
}

```

## Contributing

Create a pull request.

## Thanks

- To the AWS Lambda team for their Lambda handler function, which greatly inspired this code.

## License

[MIT](./LICENSE)