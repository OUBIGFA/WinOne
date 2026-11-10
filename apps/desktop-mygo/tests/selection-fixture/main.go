// selection-fixture uses the same MyGo inputs and selectable text as WinOne.
// It runs only on the explicitly selected, isolated test desktop.
package main

import (
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func main() {
	value := "Before WinOneSelection after"
	mygo.App.WhenReady(func(){
		mygo.NewWindow(mygo.WindowOptions{Title:"WinOne selection native fixture",Width:650,Height:300,Content:ui.View(func(c *ui.Context){
			ui.Column(c).Padding(24).Gap(20).Children(func(){
				ui.TextInput(c,&value).Width(500).Label("Selection input")
				ui.Column(c).Selectable().Children(func(){ui.Text(c,"WinOne paragraph selection").FontSize(18)})
			})
		})})
	})
	mygo.App.Run()
}
