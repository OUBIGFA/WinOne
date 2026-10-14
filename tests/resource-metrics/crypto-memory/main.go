// A standard-library-only diagnostic for the cost of crypto/rand. It is not
// linked into WinOne and never prints random bytes or reads user configuration.
package main

import (
	"crypto/fips140"
	"crypto/rand"
	"encoding/json"
	"os"
	"runtime"
)

func main() {
	var value [1]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(struct {
		Ready       bool   `json:"ready"`
		FIPSEnabled bool   `json:"fipsEnabled"`
		GoVersion   string `json:"goVersion"`
	}{true, fips140.Enabled(), runtime.Version()}); err != nil {
		panic(err)
	}
	// The measurement owner closes stdin to request normal shutdown.
	_, _ = os.Stdin.Read(value[:])
}
