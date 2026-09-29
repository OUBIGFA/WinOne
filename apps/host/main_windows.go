//go:build windows

package main

import (
	"context"
	"flag"
	"fmt"
	"fyne.io/systray"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"winone/internal/branding"

	"winone/internal/host"
	"winone/internal/modules"
	"winone/internal/settings"
	"winone/internal/transport"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	pipeDefault, err := transport.PipeName()
	if err != nil {
		return err
	}
	pipe := flag.String("pipe", pipeDefault, "local pipe name")
	data := flag.String("data-dir", filepath.Join(os.Getenv("LOCALAPPDATA"), "WinOne"), "settings directory")
	headless := flag.Bool("headless", false, "run without tray for integration tests")
	autorun := flag.Bool("autorun", false, "run configured login tasks")
	flag.Parse()
	listener, err := transport.Listen(*pipe)
	if err != nil {
		return fmt.Errorf("start Host: %w", err)
	}
	defer listener.Close()
	store, err := settings.Open(filepath.Join(*data, "settings.json"))
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	if *headless {
		return transport.Serve(ctx, listener, host.New(store))
	}
	service := host.New(store)
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	wintray := modules.Start(ctx, "wintray", "启动与托盘", filepath.Join(filepath.Dir(executable), "WinOne.WinTray.exe"), *pipe+"-wintray", filepath.Join(*data, "modules", "wintray"), *autorun)
	service.RegisterModule("wintray", wintray.Status, wintray.Handle)
	capture := modules.NewLazy(ctx, "capture", "截图", filepath.Join(filepath.Dir(executable), "capture", "WinOne.Capture.exe"), *pipe+"-capture", filepath.Join(*data, "modules", "capture"))
	service.RegisterModule("capture", capture.Status, capture.Handle)
	result := make(chan error, 1)
	go func() { result <- transport.Serve(ctx, listener, service); systray.Quit() }()
	systray.Run(func() {
		systray.SetIcon(branding.Icon)
		systray.SetTooltip("WinOne")
		open := systray.AddMenuItem("打开 WinOne", "打开设置")
		collected := systray.AddMenuItem("收纳的托盘图标", "程序托盘入口")
		collected.Hide()
		go syncCollectedMenu(ctx, collected, wintray)
		systray.AddSeparator()
		quit := systray.AddMenuItem("退出 WinOne", "结束后台")
		go func() {
			for {
				select {
				case <-open.ClickedCh:
					exe, err := os.Executable()
					if err != nil {
						fmt.Fprintln(os.Stderr, err)
						continue
					}
					cmd := exec.Command(filepath.Join(filepath.Dir(exe), "WinOne.exe"), "--pipe", *pipe, "--data-dir", *data)
					// HideConsole prevents a terminal without requesting SW_HIDE for the GUI.
					cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
					if err = cmd.Start(); err != nil {
						fmt.Fprintln(os.Stderr, err)
					} else {
						go func() { _ = cmd.Wait() }()
					}
				case <-quit.ClickedCh:
					if err := capture.Close(); err != nil {
						fmt.Fprintln(os.Stderr, err)
						systray.SetTooltip(err.Error())
						continue
					}
					if err := wintray.Close(); err != nil {
						capture.Resume()
						fmt.Fprintln(os.Stderr, err)
						systray.SetTooltip(err.Error())
						continue
					}
					cancel()
					systray.Quit()
					return
				case <-ctx.Done():
					systray.Quit()
					return
				}
			}
		}()
	}, func() { cancel() })
	return <-result
}
