// Command hostcall is a test-only client for an isolated Host pipe: the real-window
// checks use it to prepare state they then read in the window. It is not packaged.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"winone/desktopmygo/host"
)

func main() {
	if len(os.Args) != 4 {
		fmt.Fprintln(os.Stderr, "usage: hostcall <pipe> <method> <json params>")
		os.Exit(2)
	}
	client, err := host.NewClient(os.Args[1])
	if err == nil {
		var value json.RawMessage
		if value, err = client.Call(os.Args[2], json.RawMessage(os.Args[3])); err == nil {
			fmt.Println(string(value))
			return
		}
	}
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
