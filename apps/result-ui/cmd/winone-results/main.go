package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"golang.org/x/sys/windows"
	"os"
	"runtime/debug"
	"winone/internal/catalog"
	"winone/internal/processlifetime"
	"winone/internal/protocol"
	"winone/internal/resultwindow"
	"winone/internal/startupprobe"
	"winone/resultui"
)

func readCommand() (resultwindow.Command, error) {
	data, err := protocol.ReadFrame(os.Stdin)
	if err != nil {
		return resultwindow.Command{}, err
	}
	var command resultwindow.Command
	err = json.Unmarshal(data, &command)
	return command, err
}
func run() error {
	first, err := readCommand()
	if err != nil {
		return err
	}
	if first.Kind != "open" || first.Initial == nil {
		return errors.New("start the result window from WinOne")
	}
	initial := first.Initial
	if initial.HostPID == 0 || initial.Pipe == "" {
		return errors.New("invalid parent")
	}
	parent, err := windows.OpenProcess(windows.SYNCHRONIZE, false, initial.HostPID)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(parent)
	client := &resultui.Client{Pipe: initial.Pipe, HostPID: initial.HostPID, Parent: parent}
	// Same native controls and text engine, using the renderer measured for
	// this small text surface. No hidden WebView or resident warm-up is started.
	if _, set := os.LookupEnv("MYGO_GPU"); !set {
		_ = os.Setenv("MYGO_GPU", "0")
	}
	// Short-lived layout allocations accumulated after real interaction.
	// GOGC 50 reduced the measured result working set without a hard heap
	// limit; leave Host/AI runtimes and an explicit user override untouched.
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(50)
	}
	mygo.App.SetName("WinOne")
	defer resultui.ShutdownSpeech()
	controllers := map[string]*resultui.Controller{}
	open := func(input *resultwindow.Initial) {
		if input == nil || input.HostPID != initial.HostPID || input.Pipe != initial.Pipe || (input.Mode != "ocr" && input.Mode != "translate" && input.Mode != "chat") {
			return
		}
		if _, exists := controllers[input.ID]; exists {
			return
		}
		controller := resultui.NewController(client, *input)
		// Event closures retain this launch descriptor for window identity only.
		// Keep no second copy of Quick drafts or source content after handoff.
		input.Text = ""
		input.State = nil
		width, height, minWidth, minHeight := resultui.DefaultResultWidth, resultui.DefaultResultHeight, 320, 300
		if controller.Toolbar {
			width, height = controller.ToolbarDimensions()
			minWidth, minHeight = 0, 0
		}
		// A transient result floats above the capture it replaces. Pinning
		// keeps it open when focus leaves; it does not create a second runtime.
		window := mygo.NewWindow(mygo.WindowOptions{Title: "WinOne", Width: width, Height: height, UseContentSize: true, MinWidth: minWidth, MinHeight: minHeight, Frameless: true, Hidden: true, AlwaysOnTop: input.Source != "quick-assistant" || controller.Panel.Pinned, SkipTaskbar: controller.SkipTaskbar(), DisableResize: controller.Toolbar, DisableMaximize: true, Content: ui.View(controller.View)})
		controller.Window = window
		controller.Panel.Actions.Pin = window.SetAlwaysOnTop
		controllers[input.ID] = controller
		prepare := func() error {
			if err := controller.PrepareFrame(); err != nil {
				return err
			}
			if input.Source == "selection" {
				return resultui.PlaceSelectionWindow(window.NativeHandle(), input.Anchor)
			}
			return nil
		}
		if err := prepare(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			controller.Closed()
			delete(controllers, input.ID)
			window.Close()
			return
		}
		window.SetIcon(resultui.Logo())
		window.OnFocus(func() { controller.KeepOpen(); controller.RefreshServices() })
		window.OnBlur(func() {
			// The toolbar is deliberately shown without activation. Its lifetime
			// follows the Host's outside-input/focus watcher, not a blur emitted
			// while Windows is creating or exposing this inactive tool window.
			// A result and its selection toolbar form one interaction. Keep the
			// source while focus moves within it; outside focus closes unpinned results.
			foreground := resultui.ForegroundResultWindow()
			for _, c := range controllers {
				if c.Window.NativeHandle() == foreground {
					return
				}
			}
			for _, c := range controllers {
				if !c.Toolbar && !c.Panel.Pinned {
					c.CloseWindow()
				}
			}
		})
		window.OnClosed(func() { controller.Closed(); delete(controllers, input.ID) })
		// Show once without competing for activation. The foreground capture
		// process activates this verified HWND before closing its own canvas.
		if input.Source == "quick-assistant" {
			window.Show()
		} else {
			window.ShowInactive()
		}
		controller.Ready()
	}
	mygo.App.WhenReady(func() { open(initial) })
	go func() {
		for {
			command, err := readCommand()
			if err != nil {
				mygo.App.Quit()
				return
			}
			mygo.RunOnMain(func() {
				switch command.Kind {
				case "activate":
					if c := controllers[command.ID]; c != nil {
						c.KeepOpen()
						if command.Initial != nil {
							c.Initial.Actions = command.Initial.Actions
							c.Panel.QuickReasoning = command.Initial.Quick.ShowReasoning
							c.Panel.QuickActions = resultwindow.QuickAssistantActions(command.Initial.Actions)
							c.ReceiveQuickInput(command.Initial.Text)
							command.Initial.Text = ""
							command.Initial.State = nil
						}
						c.Panel.FocusComposer = true
						c.Window.Show()
					}
				case "read-selection":
					if c := controllers[command.ID]; c != nil {
						c.ReadSelection(command.Request)
					}
				case "open":
					open(command.Initial)
				case "close":
					if c := controllers[command.ID]; c != nil {
						c.CloseWindow()
					}
				case "quit":
					mygo.App.Quit()
				case "dismiss-selection":
					for _, c := range controllers {
						c.DismissToolbar(command.Point, command.Force, command.Window)
					}
				}
			})
		}
	}()
	return mygo.App.Run()
}
func main() {
	if len(os.Args) == 3 && os.Args[1] == "--check-startup" {
		executable, err := os.Executable()
		if err == nil {
			err = json.NewEncoder(os.Stdout).Encode(startupprobe.Report{Protocol: 1, Nonce: os.Args[2], PID: os.Getpid(), Executable: executable, Modules: catalog.Builtins()})
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			processlifetime.ExitAfterCleanup(1)
		}
		processlifetime.ExitAfterCleanup(0)
	}

	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "Result window:", err)
		processlifetime.ExitAfterCleanup(1)
	}
	processlifetime.ExitAfterCleanup(0)
}
