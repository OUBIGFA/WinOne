//go:build !windows

package chat

import "os"

func finishConversationCopy(source, destination string) error { return os.Link(source, destination) }
