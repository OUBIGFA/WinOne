//go:build windows

package main

import (
	"errors"
	"testing"
	"winone/internal/host"
)

type shutdownFixture struct {
	closed, resumed int
	err             error
}

func (w *shutdownFixture) Close() error { w.closed++; return w.err }
func (w *shutdownFixture) Resume()      { w.resumed++ }

type emptyShutdownHandler struct{}

func (emptyShutdownHandler) Handle(host.Request) host.Response { return host.Response{} }

func TestShutdownPreparationCanBeCancelledWithoutKeepingAdmissionLocked(t *testing.T) {
	queue := host.NewConfigurationQueue(emptyShutdownHandler{})
	first, second := &shutdownFixture{}, &shutdownFixture{}
	cancel, err := prepareModuleShutdown(queue, func() bool { return false }, first, second)
	if err != nil {
		t.Fatal(err)
	}
	if release, ok := queue.AcquireAction(); ok {
		release()
		t.Fatal("new action admitted during handoff")
	}
	cancel()
	cancel()
	if first.closed != 1 || second.closed != 1 || first.resumed != 1 || second.resumed != 1 {
		t.Fatal(first, second)
	}
	release, ok := queue.AcquireAction()
	if !ok {
		t.Fatal("cancelled handoff kept admission locked")
	}
	release()
}
func TestShutdownPreparationRestoresEarlierWorkersOnFailure(t *testing.T) {
	queue := host.NewConfigurationQueue(emptyShutdownHandler{})
	failure := errors.New("capture unfinished")
	first, second, third := &shutdownFixture{}, &shutdownFixture{err: failure}, &shutdownFixture{}
	cancel, err := prepareModuleShutdown(queue, func() bool { return false }, first, second, third)
	if !errors.Is(err, failure) || cancel != nil {
		t.Fatal(cancel != nil, err)
	}
	if first.resumed != 1 || second.resumed != 0 || third.closed != 0 {
		t.Fatal(first, second, third)
	}
	release, ok := queue.AcquireMaintenance()
	if !ok {
		t.Fatal("failed preparation retained maintenance")
	}
	release()
}
func TestShutdownPreparationDoesNotInterruptAdmittedWork(t *testing.T) {
	queue := host.NewConfigurationQueue(emptyShutdownHandler{})
	worker := &shutdownFixture{}
	release, ok := queue.AcquireAction()
	if !ok {
		t.Fatal("fixture admission")
	}
	if _, err := prepareModuleShutdown(queue, func() bool { return false }, worker); err == nil {
		t.Fatal("running action interrupted")
	}
	release()
	if _, err := prepareModuleShutdown(queue, func() bool { return true }, worker); err == nil {
		t.Fatal("module switch interrupted")
	}
	if worker.closed != 0 {
		t.Fatal("worker touched during active operation")
	}
}
