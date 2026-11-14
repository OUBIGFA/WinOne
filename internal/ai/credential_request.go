package ai

import "context"

type credentialContextKey struct{}
type credentialSetContextKey struct{}

// This set travels only over the owned worker pipe. It never enters a public
// settings reply; the upstream provider chooses the credential for each try.
func credentialRequestKeys(ctx context.Context) []RuntimeKey {
	keys, _ := ctx.Value(credentialSetContextKey{}).([]RuntimeKey)
	return keys
}

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
