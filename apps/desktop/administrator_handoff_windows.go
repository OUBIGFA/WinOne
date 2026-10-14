//go:build windows

package main

import (
	"context"
	"winone/internal/adminstartup"
)

type administratorHandoffSnapshot = adminstartup.HandoffSnapshot

func callAdministratorHandoff(ctx context.Context, pipe, expectedHost, method string, params any) (administratorHandoffSnapshot, error) {
	return adminstartup.CallHandoff(ctx, pipe, expectedHost, method, params)
}
