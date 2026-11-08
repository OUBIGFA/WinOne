package ai

import "context"

type credentialContextKey struct{}

// Request context carries only the selected slot's public identity/options.
// Keep this platform-independent: the shared translation API also builds on Unix.
func credentialRequestOptions(ctx context.Context) (string, *KeyOptions) {
	selected, ok := ctx.Value(credentialContextKey{}).(RuntimeKey)
	if !ok {
		return "", nil
	}
	return selected.ID, &selected.KeyOptions
}

func contextWithRuntimeKey(ctx context.Context, key RuntimeKey) context.Context {
	key.Key = ""
	return context.WithValue(ctx, credentialContextKey{}, key)
}
