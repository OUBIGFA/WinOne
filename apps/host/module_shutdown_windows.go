//go:build windows

package main

import (
	"errors"
	"sync"
	"winone/internal/host"
)

type shutdownWorker interface {
	Close() error
	Resume()
}

// prepareModuleShutdown closes admission before touching workers. Restart
// handoff can cancel the preparation if the replacement process is not ready;
// ordinary exit retains admission until this process terminates.
func prepareModuleShutdown(queue *host.ConfigurationQueue, controlBusy func() bool, workers ...shutdownWorker) (func(), error) {
	release, admitted := queue.AcquireMaintenance()
	if !admitted {
		return nil, errors.New("操作正在进行，请稍后重试")
	}
	if controlBusy() {
		release()
		return nil, errors.New("模块状态正在切换，请稍后重试")
	}
	closed := 0
	resume := func() {
		for index := closed - 1; index >= 0; index-- {
			workers[index].Resume()
		}
		release()
	}
	for _, worker := range workers {
		if err := worker.Close(); err != nil {
			resume()
			return nil, err
		}
		closed++
	}
	var once sync.Once
	return func() { once.Do(resume) }, nil
}
