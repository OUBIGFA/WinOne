package chat

import "winone/desktopmygo/chat/model"

// The native adapter follows the existing Cindy workGroupAdapter.ts: a public
// reply or plan seals the preceding activity run, only the trailing run may be
// live, and its first activity remains the expansion identity across retries.
// WinOne's explicit public-prose rule also applies to interrupted/recovered runs;
// a recovery never hides delivered prose in a completed work disclosure.
type workSegment struct {
	key      string
	response *model.Response
	entries  []model.Entry
	prose    *model.Message
	plan     *model.Plan
}

func workSegments(r *model.Response, reasoning bool) []workSegment {
	plan := model.PlanOf(r)
	var visible []model.Entry
	lastBoundary := -1
	gaps := map[int]bool{}
	var previousEnd *float64
	for _, entry := range r.Entries {
		if entry.Kind == "thinking" && !reasoning {
			continue
		}
		if entry.Kind == "tool" && model.WorkflowFromTool(entry.Tool) != nil && (plan == nil || entry.Key != plan.Anchor) {
			continue
		}
		// Cindy's historyGap.ts treats thirty minutes between loaded islands
		// as a missing-history boundary, not forty-seven hours of one task.
		if previousEnd != nil && entry.At != nil && *entry.At-*previousEnd > 30*60*1000 {
			gaps[len(visible)] = true
			lastBoundary = len(visible) - 1
		}
		end := entry.EndedAt
		if end == nil {
			end = entry.At
		}
		if end != nil && (previousEnd == nil || *end > *previousEnd) {
			previousEnd = end
		}
		if entry.Kind == "commentary" || entry.Kind == "tool" && model.WorkflowFromTool(entry.Tool) != nil || entry.Kind == "operation" && entry.Operation.Kind == "compaction" {
			lastBoundary = len(visible)
		}
		visible = append(visible, entry)
	}
	// A failed attempt is not a delivery boundary; its retry must not close or
	// remount the active latest-five preview.
	if r.Answer != nil && r.Answer.ErrorMessage == "" && r.Answer.StopReason != "error" && r.Answer.StopReason != "aborted" {
		lastBoundary = len(visible)
	}
	var segments []workSegment
	var activities []model.Entry
	previous := r.StartedAt
	groups := 0
	flush := func(next *float64, lastIndex int) {
		if len(activities) == 0 {
			return
		}
		key := r.Key
		if groups > 0 {
			key += "/" + activities[0].Key
		}
		groups++
		start := activities[0].At
		if previous != nil && (start == nil || *previous <= *start) {
			start = previous
		}
		end := next
		if end == nil {
			for _, entry := range activities {
				at := entry.EndedAt
				if at == nil {
					at = entry.At
				}
				if at != nil && (end == nil || *at > *end) {
					end = at
				}
			}
		}
		var duration *float64
		if start != nil && end != nil && *end >= *start {
			value := *end - *start
			duration = &value
		}
		group := &model.Response{Key: key, Running: r.Running && lastIndex > lastBoundary, StartedAt: start, DurationMs: duration}
		segments = append(segments, workSegment{key: "work:" + key, response: group, entries: activities})
		activities = nil
	}
	for index, entry := range visible {
		if gaps[index] {
			flush(nil, index-1)
			previous = nil
		}
		if entry.Kind == "commentary" {
			flush(entry.At, index-1)
			message := entry.Message
			if message == nil {
				message = &model.Message{Role: "assistant", Content: model.TextContent(entry.Text), Timestamp: entry.At}
			}
			segments = append(segments, workSegment{key: "public:" + entry.Key, prose: message})
			previous = entry.At
		} else if entry.Kind == "tool" && model.WorkflowFromTool(entry.Tool) != nil {
			flush(entry.At, index-1)
			segments = append(segments, workSegment{key: "plan:" + plan.Anchor, plan: plan})
			previous = entry.At
		} else {
			activities = append(activities, entry)
		}
	}
	var end *float64
	if r.Answer != nil {
		end = r.Answer.Timestamp
	}
	if end == nil && r.StartedAt != nil && r.DurationMs != nil {
		value := *r.StartedAt + *r.DurationMs
		end = &value
	}
	flush(end, len(visible)-1)
	return segments
}
