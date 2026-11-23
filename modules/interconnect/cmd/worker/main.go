package main

import (
	"os"
	"winone/interconnect/peer"
	"winone/interconnect/transfer"
)

func main() {
	if len(os.Args) != 2 {
		os.Exit(2)
	}
	switch os.Args[1] {
	case "transfer":
		transfer.Run()
	case "peer":
		peer.Run()
	default:
		os.Exit(2)
	}
}
