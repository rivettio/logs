### logs

Log Printing and Recording Tool.

### Installation

`go get github.com/rivettio/logs`

### Usage

demo:

```go
package main

import (
	"fmt"
	"github.com/rivettio/logs"
)

func main() {
	logs.Error("logs")
}

func init() {
	err := logs.Init("./", "file_log", logs.DEBUG, true, true, true)
	if err != nil {
		fmt.Println("logs init error : ", err)
	} else {
		fmt.Println("logs init success")
	}
}

```

