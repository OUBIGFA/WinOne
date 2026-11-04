// Command native-automation is a test-only client for MyGo's upstream
// automation socket. Used by build/test-native-filedrop.ps1 to read real
// native controls; file drops themselves come from Windows Explorer.
package main

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	socket := flag.String("socket", "", "task-local socket")
	method := flag.String("method", "tree", "upstream automation method")
	params := flag.String("params", "{}", "JSON params")
	paramsFile := flag.String("params-file", "", "optional task-local JSON params file")
	output := flag.String("out", "", "optional task-local result file")
	flag.Parse()
	if !filepath.IsAbs(*socket) || !strings.Contains(filepath.ToSlash(*socket), "/_temp/") {
		fatal(fmt.Errorf("use a task-local absolute socket"))
	}
	if *output != "" && (!filepath.IsAbs(*output) || !strings.Contains(filepath.ToSlash(*output), "/_temp/")) {
		fatal(fmt.Errorf("use a task-local output"))
	}
	var args any
	if *paramsFile != "" {
		if !filepath.IsAbs(*paramsFile) || !strings.Contains(filepath.ToSlash(*paramsFile), "/_temp/") {
			fatal(fmt.Errorf("use a task-local params file"))
		}
		data, err := os.ReadFile(*paramsFile)
		if err != nil {
			fatal(err)
		}
		*params = string(data)
	}
	if err := json.Unmarshal([]byte(*params), &args); err != nil {
		fatal(err)
	}
	conn, err := net.DialTimeout("unix", *socket, 5*time.Second)
	if err != nil {
		fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(15 * time.Second))
	data, _ := json.Marshal(map[string]any{"id": 1, "method": *method, "params": args})
	if _, err = conn.Write(append(data, '\n')); err != nil {
		fatal(err)
	}
	data, err = bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		fatal(err)
	}
	var result struct {
		Error  json.RawMessage
		Result json.RawMessage
	}
	if err = json.Unmarshal(data, &result); err != nil {
		fatal(err)
	}
	if len(result.Error) > 0 {
		fatal(fmt.Errorf("%s", result.Error))
	}
	if *output != "" {
		if *method == "capture" {
			var image struct{ PNG string }
			if err = json.Unmarshal(result.Result, &image); err != nil {
				fatal(err)
			}
			data, err = base64.StdEncoding.DecodeString(image.PNG)
			if err != nil {
				fatal(err)
			}
		} else {
			data = result.Result
		}
		if err = os.WriteFile(*output, data, 0600); err != nil {
			fatal(err)
		}
	} else {
		fmt.Println(string(result.Result))
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
