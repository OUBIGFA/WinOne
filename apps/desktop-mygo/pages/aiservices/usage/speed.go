package usage

// requestSpeed is ported from yetone/magpie at
// c4a5e61530cdd979369721826eca14ff86f426ee, plugins/usage/page.js
// (ledFloor_ and ledNoSpeed_, the gateway's own requests), as
// apps/desktop/frontend/src/features/ai-request-speed.ts carried it. MIT
// License, Copyright (c) 2026 yetone; the notice is
// third_party/licenses/upstream-magpie-c4a5e61530cdd979369721826eca14ff86f426ee-0-LICENSE.txt.
//
// The Host gives a request's measured speed; without it, a request may
// still tell the least it wrote at, with the sampling thresholds upstream
// keeps. Floors belong to rows, never to the aggregate speed figures.

// speed is a request's speed, or why it has none: reason is the i18n key
// suffix ("usage."+reason) explaining a floor or its absence.
type speed struct {
	value  float64
	floor  bool
	reason string
}

func requestSpeed(r row) speed {
	none := func(reason string) speed { return speed{reason: reason} }
	if r.failed() {
		return none("speedFailed")
	}
	if r.Speed > 0 {
		return speed{value: r.Speed}
	}
	first, reasoning, firstText, flow := r.TTFT, valueOf(r.Reasoning), r.FirstText, r.Flow
	answer, window := r.Out-reasoning, r.Ms-first
	if first > 0 && reasoning > 0 && !(firstText > 0) && answer >= 100 && window >= 1000 && answer*1000 <= 10000*window && !(flow > 0 && flow*4 < window) {
		return speed{value: answer * 1000 / window, floor: true, reason: "speedAnswerFloor"}
	}
	if r.Out >= 1000 && r.Ms >= 1000 && r.Out*1000 <= 10000*r.Ms {
		return speed{value: r.Out * 1000 / r.Ms, floor: true, reason: "speedWholeFloor"}
	}
	if !(r.Ms > 0) {
		return none("speedNoDuration")
	}
	if !(r.Out > 0) {
		return none("speedNoOutput")
	}
	if !(first > 0) {
		return none("speedNoFirstToken")
	}
	if !(answer > 0) {
		return none("speedReasoningOnly")
	}
	from := first
	if reasoning > 0 && firstText > 0 {
		from = firstText
	}
	w := r.Ms - from
	if w >= 100 && (answer*1000 > 10000*w || flow > 0 && flow*4 < w || reasoning > 0 && firstText > 0 && answer*(firstText-first) > 20*reasoning*w) {
		return none("speedBurst")
	}
	return none("speedTooLittle")
}
