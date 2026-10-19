package chat

// The usage reader visits only session files already registered by WinOne.
// It never starts Pi, loads extensions, changes the active branch or returns
// message bodies. Token counts are the provider's persisted usage, not guesses.
import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"winone/internal/ai"
)

type UsageQuery struct {
	Period   string `json:"period"`
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Project  string `json:"project"`
	Query    string `json:"query"`
	ID       string `json:"id"`
	Offset   int    `json:"offset"`
	TopBy    string `json:"topBy"`
}
type UsageCounts struct {
	Sessions    int     `json:"sessions"`
	Messages    int     `json:"messages"`
	Calls       int     `json:"calls"`
	Input       int64   `json:"input"`
	Output      int64   `json:"output"`
	CacheRead   int64   `json:"cache_read"`
	CacheWrite  int64   `json:"cache_write"`
	Reasoning   int64   `json:"reasoning"`
	Errors      int     `json:"errors"`
	Compactions int     `json:"compactions"`
	ActiveMS    int64   `json:"activeMs"`
	Cost        float64 `json:"cost"`
	Unpriced    int     `json:"unpriced"`
}

func (u *UsageCounts) add(v UsageCounts) {
	u.Sessions += v.Sessions
	u.Messages += v.Messages
	u.Calls += v.Calls
	u.Input += v.Input
	u.Output += v.Output
	u.CacheRead += v.CacheRead
	u.CacheWrite += v.CacheWrite
	u.Reasoning += v.Reasoning
	u.Errors += v.Errors
	u.Compactions += v.Compactions
	u.ActiveMS += v.ActiveMS
	u.Cost += v.Cost
	u.Unpriced += v.Unpriced
}

type UsagePoint struct {
	Time string `json:"time"`
	UsageCounts
}
type ContextPoint struct {
	Time       string `json:"time"`
	Input      int64  `json:"input"`
	CacheRead  int64  `json:"cache_read"`
	Output     int64  `json:"output"`
	Compaction bool   `json:"compaction,omitempty"`
}
type UsageSession struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Project   string `json:"project"`
	Provider  string `json:"provider"`
	Model     string `json:"model"`
	CreatedAt string `json:"createdAt"`
	UpdatedAt string `json:"updatedAt"`
	Archived  bool   `json:"archived"`
	UsageCounts
	Context []ContextPoint         `json:"context"`
	Tools   map[string]int         `json:"tools"`
	Skills  map[string]int         `json:"skills"`
	Roles   map[string]int         `json:"roles"`
	Hours   map[string]UsageCounts `json:"hours"`
	Error   string                 `json:"error,omitempty"`
	Partial bool                   `json:"partial,omitempty"`
}
type UsageReport struct {
	Rows     []UsageSession                    `json:"rows"`
	Sum      UsageCounts                       `json:"sum"`
	Series   []UsagePoint                      `json:"series"`
	By       map[string]map[string]UsageCounts `json:"by"`
	Total    int                               `json:"total"`
	Offset   int                               `json:"offset"`
	Limit    int                               `json:"limit"`
	Median   int64                             `json:"median"`
	P90      int64                             `json:"p90"`
	Top      []UsageSession                    `json:"top"`
	Tools    map[string]int                    `json:"tools"`
	Skills   map[string]int                    `json:"skills"`
	Warnings []string                          `json:"warnings"`
	Shape    map[string]UsageShape             `json:"shape"`
}
type UsageShape struct {
	Edges  []int `json:"edges"`
	Counts []int `json:"counts"`
	Total  int   `json:"total"`
}

// Magpie internal/sessions/stats.go shapes(), a8ca790 (MIT), adapted to Pi's
// persisted roles and WinOne's millisecond activity measure.
func usageShapes(in []UsageSession) map[string]UsageShape {
	out := map[string]UsageShape{}
	for by, edges := range map[string][]int{"messages": {1, 6, 16, 31, 61, 121}, "minutes": {1, 6, 16, 31, 61, 121}, "autonomy": {0, 1, 3, 6, 11, 21}} {
		shape := UsageShape{Edges: edges, Counts: make([]int, len(edges))}
		for _, s := range in {
			v := s.Roles["user"] + s.Roles["assistant"]
			switch by {
			case "minutes":
				if s.ActiveMS <= 0 {
					continue
				}
				v = int(max(1, (s.ActiveMS+30000)/60000))
			case "messages", "autonomy":
				if v == 0 {
					continue
				}
			}
			if by == "autonomy" {
				calls := 0
				for _, n := range s.Tools {
					calls += n
				}
				v = calls / max(1, s.Roles["user"])
			}
			i := len(edges) - 1
			for i > 0 && v < edges[i] {
				i--
			}
			shape.Counts[i]++
			shape.Total++
		}
		out[by] = shape
	}
	return out
}

type UsagePrice struct {
	Cost        float64            `json:"cost"`
	PricedCalls int                `json:"pricedCalls"`
	Days        map[string]float64 `json:"days"`
}

func (s *Store) Usage(q UsageQuery, pricing ...map[string]UsagePrice) (UsageReport, error) {
	report := UsageReport{Rows: []UsageSession{}, Series: []UsagePoint{}, By: map[string]map[string]UsageCounts{}, Limit: 15, Offset: q.Offset, Top: []UsageSession{}, Tools: map[string]int{}, Skills: map[string]int{}, Warnings: []string{}}
	since, until, err := ai.UsagePeriodRange(q.Period, time.Now())
	if err != nil {
		return report, err
	}
	if q.Offset < 0 || q.Offset > 1000000 || len(q.Query) > 512 || len(q.Model) > 512 || len(q.Provider) > 256 || len(q.Project) > 256 {
		return report, errors.New("会话筛选条件无效")
	}
	if q.ID != "" && !identifier.MatchString(q.ID) || q.TopBy != "" && q.TopBy != "tokens" && q.TopBy != "cost" && q.TopBy != "active" {
		return report, errors.New("会话筛选条件无效")
	}
	data := s.Snapshot()
	days := map[string]UsageCounts{}
	// Keep the report arrays non-nil: a nil Go slice serializes as JSON null,
	// and the settings UI reads rows, top and series as arrays.
	matched := []UsageSession{}
	for _, c := range data.Conversations {
		if q.ID != "" && q.ID != c.ID || q.Provider != "" && q.Provider != c.Model.Provider || q.Model != "" && q.Model != c.Model.Model || q.Project != "" && q.Project != c.ProjectID {
			continue
		}
		if q.Query != "" && !strings.Contains(strings.ToLower(c.Title+" "+c.Model.Model), strings.ToLower(q.Query)) {
			continue
		}
		u := UsageSession{ID: c.ID, Title: c.Title, Project: c.ProjectID, Provider: c.Model.Provider, Model: c.Model.Model, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt, Archived: c.Archived, Context: []ContextPoint{}, Tools: map[string]int{}, Skills: map[string]int{}, Roles: map[string]int{}, Hours: map[string]UsageCounts{}}
		activity, readErr := readUsageSession(c, since, until, &u)
		if readErr != nil {
			u.Error = "会话统计无法读取"
			report.Warnings = append(report.Warnings, c.Title+"："+u.Error)
		}
		updated, _ := time.Parse(time.RFC3339Nano, c.UpdatedAt)
		if len(activity) == 0 && readErr == nil && (updated.Before(since) || !until.IsZero() && !updated.Before(until)) {
			continue
		}
		u.Sessions = 1
		u.Unpriced = u.Calls
		if len(pricing) > 0 {
			if price, ok := pricing[0][c.ID]; ok {
				u.Cost = price.Cost
				u.Unpriced = max(0, u.Calls-price.PricedCalls)
				for day, cost := range price.Days {
					counts := activity[day]
					counts.Cost = cost
					activity[day] = counts
				}
			}
		}
		report.Sum.add(u.UsageCounts)
		matched = append(matched, u)
		for key, value := range activity {
			value.Sessions = 1
			old := days[key]
			old.add(value)
			days[key] = old
		}
		for dimension, key := range map[string]string{"model": u.Model, "provider": u.Provider, "project": u.Project} {
			if report.By[dimension] == nil {
				report.By[dimension] = map[string]UsageCounts{}
			}
			group := report.By[dimension][key]
			group.add(u.UsageCounts)
			report.By[dimension][key] = group
		}
		for key, n := range u.Tools {
			report.Tools[key] += n
		}
		for key, n := range u.Skills {
			report.Skills[key] += n
		}
		if report.By["hour"] == nil {
			report.By["hour"] = map[string]UsageCounts{}
		}
		for hour, counts := range u.Hours {
			group := report.By["hour"][hour]
			group.add(counts)
			report.By["hour"][hour] = group
		}
	}
	slices.SortFunc(matched, func(a, b UsageSession) int { return strings.Compare(b.UpdatedAt, a.UpdatedAt) })
	report.Total = len(matched)
	report.Shape = usageShapes(matched)
	start := min(q.Offset, len(matched))
	report.Rows = matched[start:min(start+15, len(matched))]
	for day, counts := range days {
		report.Series = append(report.Series, UsagePoint{Time: day, UsageCounts: counts})
	}
	slices.SortFunc(report.Series, func(a, b UsagePoint) int { return strings.Compare(a.Time, b.Time) })
	var lengths []int64
	for _, u := range matched {
		lengths = append(lengths, u.Input+u.Output+u.CacheRead+u.CacheWrite)
	}
	slices.Sort(lengths)
	if len(lengths) > 0 {
		report.Median = lengths[len(lengths)/2]
		report.P90 = lengths[min(len(lengths)-1, (len(lengths)*9)/10)]
	}
	report.Top = slices.Clone(matched)
	slices.SortFunc(report.Top, func(a, b UsageSession) int {
		if q.TopBy == "cost" {
			if a.Cost > b.Cost {
				return -1
			}
			if a.Cost < b.Cost {
				return 1
			}
		}
		if q.TopBy == "active" {
			if a.ActiveMS > b.ActiveMS {
				return -1
			}
			if a.ActiveMS < b.ActiveMS {
				return 1
			}
		}
		av, bv := a.Input+a.Output+a.CacheRead+a.CacheWrite, b.Input+b.Output+b.CacheRead+b.CacheWrite
		if av > bv {
			return -1
		}
		if av < bv {
			return 1
		}
		return 0
	})
	report.Top = report.Top[:min(15, len(report.Top))]
	return report, nil
}

type piUsage struct {
	Input      int64 `json:"input"`
	Output     int64 `json:"output"`
	CacheRead  int64 `json:"cacheRead"`
	CacheWrite int64 `json:"cacheWrite"`
	Reasoning  int64 `json:"reasoning"`
}
type piUsageEntry struct {
	Type         string   `json:"type"`
	Timestamp    string   `json:"timestamp"`
	TokensBefore int64    `json:"tokensBefore"`
	Usage        *piUsage `json:"usage"`
	Message      struct {
		Role       string          `json:"role"`
		Usage      *piUsage        `json:"usage"`
		StopReason string          `json:"stopReason"`
		ToolName   string          `json:"toolName"`
		IsError    bool            `json:"isError"`
		Content    json.RawMessage `json:"content"`
	} `json:"message"`
}

func readUsageSession(c Conversation, since, until time.Time, out *UsageSession) (map[string]UsageCounts, error) {
	days := map[string]UsageCounts{}
	if c.SessionFile == "" {
		return days, nil
	}
	if !within(c.SessionDirectory, c.SessionFile) {
		return days, errors.New("会话路径越界")
	}
	root, err := filepath.EvalSymlinks(c.SessionDirectory)
	if err != nil {
		return days, err
	}
	file, err := filepath.EvalSymlinks(c.SessionFile)
	if err != nil {
		return days, err
	}
	if !within(root, file) {
		return days, errors.New("会话路径越界")
	}
	f, err := os.Open(file)
	if err != nil {
		return days, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return days, err
	}
	if info.Size() > 128<<20 {
		return days, errors.New("会话文件过大")
	}
	reader := bufio.NewReader(io.LimitReader(f, 128<<20))
	var previous time.Time
	for {
		raw, readErr := reader.ReadBytes('\n')
		if len(bytes.TrimSpace(raw)) > 0 {
			var entry piUsageEntry
			if err := json.Unmarshal(raw, &entry); err != nil {
				if readErr == io.EOF {
					out.Partial = true
					break
				}
				return days, err
			}
			at, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
			if err != nil && entry.Type != "session" {
				return days, errors.New("会话时间无效")
			}
			if !at.Before(since) && (until.IsZero() || at.Before(until)) && entry.Type != "session" {
				counts := UsageCounts{}
				if entry.Type == "message" {
					counts.Messages = 1
					out.Roles[entry.Message.Role]++
					if entry.Message.IsError || entry.Message.StopReason == "error" {
						counts.Errors++
					}
				}
				used := entry.Usage
				if entry.Message.Usage != nil {
					used = entry.Message.Usage
				}
				if used != nil {
					counts.Calls = 1
					counts.Input = max(0, used.Input)
					counts.Output = max(0, used.Output)
					counts.CacheRead = max(0, used.CacheRead)
					counts.CacheWrite = max(0, used.CacheWrite)
					counts.Reasoning = max(0, used.Reasoning)
					out.Context = append(out.Context, ContextPoint{Time: entry.Timestamp, Input: counts.Input + counts.CacheRead + counts.CacheWrite, CacheRead: counts.CacheRead, Output: counts.Output})
				}
				if entry.Type == "compaction" {
					counts.Compactions++
					out.Context = append(out.Context, ContextPoint{Time: entry.Timestamp, Input: entry.TokensBefore, Compaction: true})
				}
				// Session activity uses adjacent message timestamps, capped at five
				// minutes so time away from the chat is not presented as active work.
				if entry.Type == "message" && !previous.IsZero() {
					gap := at.Sub(previous)
					if gap > 0 && gap <= 5*time.Minute {
						counts.ActiveMS = gap.Milliseconds()
					}
				}
				if entry.Type == "message" {
					previous = at
				}
				var parts []struct {
					Type      string `json:"type"`
					Name      string `json:"name"`
					Arguments struct {
						Path string `json:"path"`
					} `json:"arguments"`
				}
				if json.Unmarshal(entry.Message.Content, &parts) == nil {
					for _, part := range parts {
						if part.Type == "toolCall" && part.Name != "" {
							out.Tools[part.Name]++
							if part.Name == "read" && strings.EqualFold(filepath.Base(part.Arguments.Path), "SKILL.md") {
								out.Skills[filepath.Base(filepath.Dir(part.Arguments.Path))]++
							}
						}
					}
				}
				out.UsageCounts.add(counts)
				hour := at.Local().Format("15")
				hourCounts := out.Hours[hour]
				hourCounts.add(counts)
				out.Hours[hour] = hourCounts
				key := at.Local().Format("2006-01-02")
				day := days[key]
				day.add(counts)
				days[key] = day
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return days, readErr
		}
	}
	return days, nil
}
