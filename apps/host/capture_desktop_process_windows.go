//go:build windows

package main

import (
	"context"
	"errors"
	"winone/internal/capturehandoff"
	"winone/internal/processidentity"
	"winone/internal/windowinstance"
)

func existingCaptureProcess(executable, pipe string) (*processidentity.Process, error) {
	pid, err := windowinstance.FindPID(capturehandoff.WindowInstanceID(pipe))
	if err != nil || pid == 0 {
		return nil, err
	}
	process, err := processidentity.Open(pid, executable)
	if errors.Is(err, processidentity.ErrExited) {
		return nil, nil
	}
	return process, err
}

func startCaptureWindow(executable, pipe, directory, draft string) (<-chan error, error) {
	existing, err := existingCaptureProcess(executable, pipe)
	if err != nil {
		return nil, err
	}
	done := make(chan error, 1)
	if existing != nil {
		go func() { defer existing.Close(); done <- existing.Wait(context.Background()); close(done) }()
		return done, nil
	}
	cmd, err := startDesktop(executable, pipe, directory, draft)
	if err != nil {
		return nil, err
	}
	go func() {
		err := cmd.Wait()
		// Another launcher can win the single-instance race. In that case this
		// command merely notifies the real window; observe that window's process.
		if err == nil {
			var actual *processidentity.Process
			actual, err = existingCaptureProcess(executable, pipe)
			if actual != nil {
				err = actual.Wait(context.Background())
				actual.Close()
			}
		}
		done <- err
		close(done)
	}()
	return done, nil
}
