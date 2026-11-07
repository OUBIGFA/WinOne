// Command native-ui-matrix is built only by build/test-native-ui-matrix.ps1.
// It runs the unmodified production application, while its fixture-client mode
// uses the same test executable identity the real Host requires. No Host guard
// is disabled and no alternate UI or service implementation is supplied.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	_ "github.com/egoist/mygo/plugins/automation"
	"winone/desktopmygo/app"
	"winone/desktopmygo/host"
	_ "winone/desktopmygo/pages"
)

func main() {
	if len(os.Args) == 5 && os.Args[1] == "--hostcall" {
		client, err := host.NewClient(os.Args[2])
		if err == nil {
			defer client.Cancel()
			var raw json.RawMessage
			raw, err = client.Call(os.Args[3], json.RawMessage(os.Args[4]))
			if err == nil {
				fmt.Println(string(raw))
				return
			}
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(app.Main(os.Args[1:]))
}
