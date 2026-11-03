package webui

import "strings"

// The Host methods the pages may call, as the Rust window's bridge allowed
// them (apps/desktop-rust/src/component_bridge.rs: module_method for
// "chat", onebot_method). Generated from that source; keep them in step.

func set(values ...string) map[string]bool {
	m := make(map[string]bool, len(values))
	for _, v := range values {
		m[v] = true
	}
	return m
}

// chatAllowed reports whether a page may call chat.<method>: Chat's own,
// a OneBot teammate's conversation under onebot.chat., or OneBot's.
func chatAllowed(method string) bool {
	if rest, ok := strings.CutPrefix(method, "onebot.chat."); ok {
		return chatMethods[rest]
	}
	if rest, ok := strings.CutPrefix(method, "onebot."); ok {
		return onebotMethods[rest] || strings.HasPrefix(rest, "voice.")
	}
	return chatMethods[method]
}

var chatMethods = set(
	"file.resolve", "file.stat", "files.list", "files.index", "files.search", "files.cancel",
	"thinking", "fast", "agents", "file.preview", "media.source", "media.upload", "media.attach",
	"file.save", "file.create", "file.open", "file.reveal", "terminal.create", "terminal.poll",
	"terminal.write", "terminal.resize", "terminal.dispose", "terminal.restart", "review.turn.apply",
	"review.turn", "review.snapshot", "review.diff", "review.diff.preview", "review.file.preview",
	"review.metadata", "review.branches", "review.commits", "review.mutate", "review.push",
	"title.generate", "title.regenerate", "project.permission", "link.open", "usage", "project.git",
	"automations.deleted", "automations.project.read", "automations.connections.get",
	"automations.templates", "automations.template.apply", "automations.list", "automations.get",
	"automations.restore", "automations.project.write", "automations.project.reconcile",
	"automations.project.promote", "automations.project.clone", "automations.project.demote",
	"automations.silence", "automations.notify", "automations.connections.save",
	"automations.template.save", "automations.template.delete", "automations.create",
	"automations.update", "automations.pause", "automations.resume", "automations.delete",
	"automations.run", "automations.runs", "automations.deleteRun", "automations.markRead",
	"automations.runtime", "automations.preview", "automations.sidebarIndexRuns",
	"automations.inflightCount", "automations.detectCwd", "automations.channels",
	"automations.capabilityStatus", "automations.hook.test", "automations.hook.generate",
	"automations.markAllRead", "automations.project.upsertSchedule",
	"automations.project.removeSchedule", "automations.open", "automations.events", "catalog",
	"activity", "settings.get", "settings.save", "resources", "files", "attachments.add",
	"attachments.upload", "automation.run", "project.add", "project.create", "project.update",
	"project.remove", "export", "conversation.new", "conversation.update", "conversation.move",
	"conversation.delete", "configure", "location", "fork", "draft.get", "draft.save", "open", "poll",
	"prompt", "queue", "edit", "retry", "state", "permission", "question", "stop", "pause", "resume",
	"compact", "release", "detach",
)

var onebotMethods = set(
	"wechat.state", "wechat.settings", "market.list", "market.status", "import.sources",
	"import.preview", "import.status", "voice.models", "voice.voices", "voice.preference",
	"voice.readPreview", "voice.readSpeech", "voice.input.data", "voice.input.history",
	"voice.input.historyForRefinement", "groups.list", "groups.get", "groups.files.describe",
	"groups.files.read", "groups.files.classify", "groups.files.resolvePath",
	"groups.files.resolvePathBatch", "routines.list", "routines.history", "routines.sources",
	"routines.nightly.get", "tasks.pr.refs", "tasks.pr.statuses", "tasks.list", "tasks.get",
	"capabilities.get", "events", "projects.list", "history.search", "profiles.list", "profiles.get",
	"profiles.versions", "profile.content.read", "bots.list", "bots.history", "avatar.source",
	"media.read", "modelChain.get", "dmThread.list", "workbench.get", "workbench.followScopes",
	"workbench.readTask", "directMessage.get", "im.list", "imConnection.list",
	"imConnection.listGroups", "imConnection.listLanes", "imConnection.listAdoptable", "memory.list",
	"memory.read", "memory.index", "memory.search", "messages.learning", "skills.list",
	"skills.query", "skills.read", "skills.readSource", "skills.mounts", "todos.read",
	"autonomy.glance", "autonomy.settings", "wechat.authorize", "wechat.cancelAuthorization",
	"wechat.unbind", "wechat.setCompanion", "wechat.setWorkingDirectory",
	"wechat.resetWorkingDirectory", "market.invite", "import.start", "bots.generateDraft",
	"bots.generateAvatar", "groups.create", "groups.update", "groups.setMembers", "groups.delete",
	"groups.send", "groups.continue", "groups.stop", "groups.plan.start", "groups.plan.dismiss",
	"groups.plan.continue", "groups.plan.retry", "groups.plan.edit", "groups.attachments.stage",
	"groups.attachments.image", "groups.files.begin", "groups.files.chunk", "groups.files.finish",
	"groups.files.image", "groups.files.discard", "groups.files.open", "groups.files.reveal",
	"groups.files.remoteImage", "groups.files.draftUrls", "routines.save", "routines.remove",
	"routines.runNow", "routines.nightly.setEnabled", "routines.nightly.setFrequency",
	"routines.nightly.runNow", "routines.nightly.revert", "routines.nightly.candidate", "tasks.stop",
	"tasks.message", "tasks.trace", "projects.set", "lifecycle.action", "session.open",
	"profiles.create", "profiles.update", "profile.content.write", "bots.createCanonicalSession",
	"bots.chooseAvatar", "modelChain.set", "modelChain.reset", "dmThread.ensure", "todos.act",
	"im.add", "im.remove", "im.reconnect", "im.online", "im.offline", "im.groups.setMode",
	"imConnection.add", "imConnection.remove", "imConnection.setGroupActivation",
	"imConnection.resetLane", "imConnection.archiveLane", "imConnection.adopt", "memory.update",
	"memory.delete", "memory.write", "memory.consolidate", "memory.importDocument", "skills.save",
	"skills.saveSource", "skills.delete", "todos.patch", "autonomy.setSettings",
	"autonomy.resetSettings", "todos.dailyCheck",
)

// chatMutates reports whether chat.<method> may change something, as the
// Rust bridge's Request.mutates did for "Chat": such a request in flight
// keeps the window from closing until it is answered.
func chatMutates(method string) bool {
	if rest, ok := strings.CutPrefix(method, "onebot.chat."); ok {
		return !chatReads[rest]
	}
	if rest, ok := strings.CutPrefix(method, "onebot."); ok {
		return !onebotReads[rest]
	}
	return !chatReads[method]
}

// chatReads are the chat methods that only read.
var chatReads = set(
	"file.resolve", "file.stat", "files.list", "files.index", "files.search", "files.cancel",
	"file.preview", "media.source", "media.upload", "terminal.poll", "terminal.resize", "review.turn",
	"review.snapshot", "review.diff", "review.diff.preview", "review.file.preview", "review.metadata",
	"review.branches", "review.commits", "usage", "project.git", "automations.deleted",
	"automations.project.read", "automations.connections.get", "automations.templates",
	"automations.template.apply", "automations.list", "automations.get", "automations.runs",
	"automations.events", "automations.runtime", "automations.preview",
	"automations.sidebarIndexRuns", "automations.inflightCount", "automations.detectCwd",
	"automations.channels", "automations.capabilityStatus", "catalog", "activity", "settings.get",
	"resources", "files", "state", "poll", "draft.get", "release", "detach",
)

// onebotReads are the onebot methods that only read; voice.* others write.
var onebotReads = set(
	"wechat.state", "wechat.settings", "market.list", "market.status", "import.sources",
	"import.preview", "import.status", "voice.models", "voice.voices", "voice.preference",
	"voice.readPreview", "voice.readSpeech", "voice.input.data", "voice.input.history",
	"voice.input.historyForRefinement", "groups.list", "groups.get", "groups.files.describe",
	"groups.files.read", "groups.files.classify", "groups.files.resolvePath",
	"groups.files.resolvePathBatch", "routines.list", "routines.history", "routines.sources",
	"routines.nightly.get", "tasks.pr.refs", "tasks.pr.statuses", "tasks.list", "tasks.get",
	"capabilities.get", "events", "projects.list", "history.search", "profiles.list", "profiles.get",
	"profiles.versions", "profile.content.read", "bots.list", "bots.history", "avatar.source",
	"media.read", "modelChain.get", "dmThread.list", "workbench.get", "workbench.followScopes",
	"workbench.readTask", "directMessage.get", "im.list", "imConnection.list",
	"imConnection.listGroups", "imConnection.listLanes", "imConnection.listAdoptable", "memory.list",
	"memory.read", "memory.index", "memory.search", "messages.learning", "skills.list",
	"skills.query", "skills.read", "skills.readSource", "skills.mounts", "todos.read",
	"autonomy.glance", "autonomy.settings",
)
