### logs

Log Printing and Recording Tool.

### Installation

`go get github.com/rivettio/logs`

### Usage


```go
package main

import (
	"github.com/rivettio/logs"
)

func main() {
	logs.Error("logs")
}

func init() {
	err := logs.Init("./", "file_log", logs.DEBUG, true, true, true)
	if err != nil {
		logs.Errorf("logs init error: %#v", err)
		return
	}

	logs.Info("logs init success")
}

```

