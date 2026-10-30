package distribution

import (
	"slices"
	"strings"
	"testing"
)

// These are the reviewed notices emitted by build-chat.ps1 and its pinned
// dependency manifest. A directory-wide text allowance would also ship secrets.
var currentChatDelivery = []string{
	"shared/chat/worker.js",
	"shared/chat/runtime.json",
	"shared/chat/scheduler.js",
	"shared/chat/script-runner.js",
	"shared/ai/runtime/photon_rs_bg.wasm",
	"shared/chat/Cindy-LICENSE.txt",
	"shared/chat/Cindy-NOTICE.txt",
	"shared/chat/JSZip-LICENSE.markdown",
	"shared/chat/licenses/chat-share-notices.json",
	"shared/chat/licenses/npm-chat-share-core-util-is-1.0.3-LICENSE.txt",
	"shared/chat/licenses/npm-chat-share-immediate-3.0.6-LICENSE.txt.txt",
	"shared/chat/licenses/npm-chat-share-inherits-2.0.4-LICENSE.txt",
	"shared/chat/licenses/npm-chat-share-isarray-1.0.0-README.md.txt",
	"shared/chat/licenses/npm-chat-share-jszip-3.10.1-LICENSE.markdown.txt",
	"shared/chat/licenses/npm-chat-share-lie-3.3.0-license.md.txt",
	"shared/chat/licenses/npm-chat-share-pako-1.0.11-LICENSE.txt",
	"shared/chat/licenses/npm-chat-share-process-nextick-args-2.0.1-license.md.txt",
	"shared/chat/licenses/npm-chat-share-readable-stream-2.3.8-LICENSE.txt",
	"shared/chat/licenses/npm-chat-share-safe-buffer-5.1.2-LICENSE.txt",
	"shared/chat/licenses/npm-chat-share-setimmediate-1.0.5-LICENSE.txt.txt",
	"shared/chat/licenses/npm-chat-share-string_decoder-1.1.1-LICENSE.txt",
	"shared/chat/licenses/npm-chat-share-util-deprecate-1.0.2-LICENSE.txt",
}

func TestChatDeliveryAcceptsCompleteReviewedResources(t *testing.T) {
	for _, name := range currentChatDelivery {
		if !allowed(name) {
			t.Errorf("Chat deliverable rejected: %s", name)
		}
	}
	for _, name := range []string{
		"shared/chat/settings.json", "shared/chat/worker.js.map",
		"shared/chat/licenses/token.txt", "shared/chat/licenses/local-debug.json",
		"shared/chat/node_modules/debug.js", "shared/chat/Cindy-LICENSE.txt.bak",
		"shared/chat/licenses/_temp/LICENSE.txt",
	} {
		if allowed(name) {
			t.Errorf("unreviewed Chat file accepted: %s", name)
		}
	}
}

func TestChatSharePayloadCannotOmitItsNoticesOrResources(t *testing.T) {
	// The new notice tree identifies this payload generation. Check the
	// lower-case keys used by both package collection and ZIP verification.
	for _, marker := range []string{"shared/chat/Cindy-LICENSE.txt", "shared/chat/licenses/chat-share-notices.json"} {
		required := requiredForPayload(map[string]bool{strings.ToLower(marker): true})
		for _, name := range currentChatDelivery {
			if !slices.Contains(required, name) {
				t.Errorf("payload with %s can omit %s", marker, name)
			}
		}
	}
}

func TestOneBotPayloadRequiresAllReviewedResources(t *testing.T) {
	for _, marker := range chatOneBotFiles {
		if !allowed(marker) {
			t.Fatalf("OneBot resource rejected: %s", marker)
		}
		required := requiredForPayload(map[string]bool{strings.ToLower(marker): true})
		for _, file := range chatOneBotFiles {
			if !slices.Contains(required, file) {
				t.Errorf("%s can omit %s", marker, file)
			}
		}
	}
	legacy := requiredForPayload(map[string]bool{"shared/chat/worker.js": true})
	for _, file := range chatOneBotFiles {
		if slices.Contains(legacy, file) {
			t.Errorf("legacy payload requires %s", file)
		}
	}
	for _, file := range []string{"shared/chat/profiles.db", "shared/chat/onebot/credentials.json", "shared/chat/cindy/config.json"} {
		if allowed(file) {
			t.Errorf("OneBot data accepted as runtime: %s", file)
		}
	}
}

func TestLegacyChatPayloadDoesNotRequireNewShareNotices(t *testing.T) {
	required := requiredForPayload(map[string]bool{"shared/chat/worker.js": true, "shared/chat/scheduler.js": true})
	for _, name := range []string{"shared/chat/Cindy-LICENSE.txt", "shared/chat/licenses/chat-share-notices.json", "shared/ai/runtime/photon_rs_bg.wasm"} {
		if slices.Contains(required, name) {
			t.Errorf("legacy rollback now requires %s", name)
		}
	}
}
