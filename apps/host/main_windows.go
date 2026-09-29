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
	result := make(chan error, 1)
	go func() { result <- transport.Serve(ctx, listener, host.New(store)); systray.Quit() }()
	systray.Run(func() {
		systray.SetIcon(branding.Icon)
		systray.SetTooltip("WinOne")
		open := systray.AddMenuItem("打开 WinOne", "打开设置")
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
