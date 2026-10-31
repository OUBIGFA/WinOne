package usage

// Magpie's own messages, in its English, with the Chinese the React window
// gave them: magpie-ledger-messages.ts, generated from yetone/magpie at
// 42ec2700bd2a4cf8d4ceb76a95767ac221e283e5 like ledger.go (MIT License,
// Copyright (c) 2026 yetone; the notice is
// third_party/licenses/upstream-magpie-42ec2700bd2a4cf8d4ceb76a95767ac221e283e5-0-LICENSE.txt),
// and WinOne's chart, heatmap and quota curve tables of ai-usage-chart.tsx,
// ai-usage-heatmap.tsx and ai-usage-quota-curve.tsx. They stay keyed by
// Magpie's English so the ported drawing code reads as upstream does;
// WinOne's own words are i18n keys.

// ledgerChinese is the ledger charts' table: the chart's overrides of
// React's chartChinese over Magpie's ledger messages.
var ledgerChinese = func() map[string]string {
	table := map[string]string{
		"model":        "模型",
		"none":         "不思考",
		"All":          "全部",
		"{n} request":  "{n} 个请求",
		"{n} requests": "{n} 个请求",
		"{n} request left out: its vendor doesn't count the cache":    "{n} 个请求未计入：其服务商不统计缓存",
		"{n} requests left out: their vendor doesn't count the cache": "{n} 个请求未计入：其服务商不统计缓存",
		"Usage trend":                       "使用趋势",
		"Other ({n})":                       "其他（{n} 项）",
		"Nothing in this period":            "这段时间没有数据",
		"No known price for these requests": "这些请求没有已知价格",
		"cost":                              "费用",
		"Other":                             "其他",
		"Tokens":                            "Token",
		"Cost":                              "费用",
		"Total":                             "合计",
		"tokens":                            "token",
		"hit rate {p}":                      "命中率 {p}",
		"calls":                             "次调用",
		"{n} failed":                        "{n} 次失败",
		"{n} looped":                        "{n} 次陷入循环",
		"Replies magpie ended because the model kept repeating itself": "magpie 因模型一直重复自己而中止的回复",
		"week of {label}": "{label} 起的一周",
		"{n} credits":     "{n} 积分",
		"What {n} of these requests cost in their plan's credits, as the replies said": "其中 {n} 次请求按回复所说消耗的套餐积分",
		"{n} tokens": "{n} token",
		"Requests":   "请求",
		"TTFT {ms}":  "首字 {ms}",
		"No streamed replies timed in this period": "这段时间没有可计时的流式回复",
		"{n} tok/s":         "{n} token/秒",
		"{k} timed reply":   "{k} 次计时",
		"{k} timed replies": "{k} 次计时",
		"{k} reply":         "{k} 次回复",
		"{k} replies":       "{k} 次回复",
		"{a}; {b}":          "{a}；{b}",
		"Measured on {k} reply, from its first token to its end; the others tell no speed":                                            "实测 {k} 次回复：从首字到结束；其余请求测不出速度",
		"Measured on {k} reply, from its first token to its end":                                                                      "实测 {k} 次回复：从首字到结束",
		"Measured on {k} replies, from their first token to their end; the others tell no speed":                                      "实测 {k} 次回复：从首字到结束；其余请求测不出速度",
		"Measured on {k} replies, from their first token to their end":                                                                "实测 {k} 次回复：从首字到结束",
		"At least: {k} reply read from a session file, which doesn't say when its answer began, so time before that is counted too":   "下限：{k} 次读自会话文件的回复。会话文件不记录正文何时开始，之前的时间也算在里面",
		"At least: {k} replies read from session files, which don't say when their answers began, so time before that is counted too": "下限：{k} 次读自会话文件的回复。会话文件不记录正文何时开始，之前的时间也算在里面",
		"credits": "积分",
	}
	// React's chart table, which it read before Magpie's.
	for key, text := range map[string]string{
		"Usage trend": "用量趋势", "Other": "其他", "Other ({n})": "其他（{n}）", "Total": "合计", "All": "全部",
		"Nothing in this period":                   "此范围内还没有请求记录。",
		"No known price for these requests":        "这些请求尚无已知价格",
		"No streamed replies timed in this period": "此范围内没有已计时的流式回复",
		"{n} request":                              "{n} 次请求", "{n} requests": "{n} 次请求", "{n} tokens": "{n} Token",
		"{n} tok/s": "{n} Token/秒", "TTFT {ms}": "首字延迟 {ms}", "hit rate {p}": "缓存命中率 {p}",
		"{n} failed": "{n} 次失败", "week of {label}": "{label} 所在周",
	} {
		table[key] = text
	}
	return table
}()

// heatChinese is the request heatmap's table.
var heatChinese = map[string]string{
	"Usage heatmap": "请求活动热图", "Tokens": "Token", "Requests": "请求数", "Cost": "费用", "Less": "少", "More": "多",
	"No requests": "没有请求", "{n} tokens": "{n} Token", "{n} request": "{n} 次请求", "{n} requests": "{n} 次请求",
}

// quotaChinese is the allowance curve's table.
var quotaChinese = map[string]string{
	"Now": "现在", "Left over time": "剩余额度趋势", "Cycle": "当前周期", "2 days": "最近两天", "Latest readings:": "最近读数：",
	"{name}: {n} left, read {when}": "{name}：剩余 {n}，读取于 {when}",
	"Solid: what was left. Dashed: an even pace, full at the cycle's start to empty at its reset; a line above its dashes lasts the cycle, one below runs out before the reset.": "实线表示实际剩余额度。虚线表示当前周期内均匀消耗的参考速度；实线低于虚线时可能在重置前用完。点击图表可放大。",
}
