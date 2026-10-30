// Command winone is WinOne.exe: the settings window drawn with MyGo's
// native UI, the Start menu's entry and the target of Explorer's menu
// entries.
package main

import (
	"os"

	"winone/desktopmygo/app"
	_ "winone/desktopmygo/pages"
)

func main() { os.Exit(app.Main(os.Args[1:])) }
