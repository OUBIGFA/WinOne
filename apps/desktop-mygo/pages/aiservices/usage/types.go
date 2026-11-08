package usage

// The documents the usage statistics read: Magpie's ledger page
// (capture.ai.usage.get), the registered Chat sessions (chat.usage), the
// context analysis that rides on the ledger read, account allowances
// (capture.ai.accounts.quota) and request archives (capture.ai.archive.read),
// as apps/desktop/frontend/src/features/ai-usage-types.ts declares them.
// Numbers are float64, as the React window read them: the ledger's sums
// are compared and divided as JavaScript numbers were. Go decodes a JSON
// null list or map as an empty one, so no normalization is needed.

// totals are Magpie's usage.Totals.
type totals struct {
	Calls             float64 `json:"calls"`
	Canceled          float64 `json:"canceled,omitempty"`
	UpBytes           float64 `json:"up_bytes,omitempty"`
	DownBytes         float64 `json:"down_bytes,omitempty"`
	ImageFilesSaved   float64 `json:"image_files_saved,omitempty"`
	Errors            float64 `json:"errors"`
	Loops             float64 `json:"loops,omitempty"`
	Input             float64 `json:"input"`
	Output            float64 `json:"output"`
	CacheRead         float64 `json:"cache_read"`
	CacheWrite        float64 `json:"cache_write"`
	CacheWrite1h      float64 `json:"cache_write_1h,omitempty"`
	CacheUnknown      float64 `json:"cache_unknown,omitempty"`
	CacheUnknownCalls float64 `json:"cache_unknown_calls,omitempty"`
	Reasoning         float64 `json:"reasoning,omitempty"`
	Cost              float64 `json:"cost"`
	Unpriced          float64 `json:"unpriced"`
	Credits           float64 `json:"credits,omitempty"`
	Credited          float64 `json:"credited,omitempty"`
	Timed             float64 `json:"timed,omitempty"`
	TTFT              float64 `json:"ttft_ms,omitempty"`
	DecodeMs          float64 `json:"decode_ms,omitempty"`
	DecodeOut         float64 `json:"decode_out,omitempty"`
	DecodeN           float64 `json:"decode_n,omitempty"`
	FloorOut          float64 `json:"floor_out,omitempty"`
	FloorMs           float64 `json:"floor_ms,omitempty"`
	FloorN            float64 `json:"floor_n,omitempty"`
}

// add sums o into t, as the ranking sums the shares it leaves out into
// "Other".
func (t *totals) add(o totals) {
	t.Calls += o.Calls
	t.Canceled += o.Canceled
	t.UpBytes += o.UpBytes
	t.DownBytes += o.DownBytes
	t.ImageFilesSaved += o.ImageFilesSaved
	t.Errors += o.Errors
	t.Loops += o.Loops
	t.Input += o.Input
	t.Output += o.Output
	t.CacheRead += o.CacheRead
	t.CacheWrite += o.CacheWrite
	t.CacheUnknown += o.CacheUnknown
	t.CacheUnknownCalls += o.CacheUnknownCalls
	t.Cost += o.Cost
	t.Credits += o.Credits
	t.Credited += o.Credited
	t.Timed += o.Timed
	t.TTFT += o.TTFT
	t.DecodeMs += o.DecodeMs
	t.DecodeOut += o.DecodeOut
	t.DecodeN += o.DecodeN
	t.FloorOut += o.FloorOut
	t.FloorMs += o.FloorMs
	t.FloorN += o.FloorN
}

// share is a group of requests by a dimension: a model, a provider, an
// agent, an account. Name is the label the page shows, filled from the
// configured providers once the page arrives.
type share struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
	totals
}

// part is a point's figures of one group of a dimension.
type part struct {
	Calls     float64 `json:"calls"`
	Tokens    float64 `json:"tokens"`
	Cost      float64 `json:"cost"`
	Timed     float64 `json:"timed,omitempty"`
	TTFT      float64 `json:"ttft_ms,omitempty"`
	DecodeMs  float64 `json:"decode_ms,omitempty"`
	DecodeOut float64 `json:"decode_out,omitempty"`
}

// point is one hour, day or week of the chart. Time is the bucket's start
// in the reader's zone (RFC 3339), its first ten characters the day.
type point struct {
	Time string `json:"time"`
	totals
	By map[string]map[string]part `json:"by,omitempty"`
}

// row is one request of the ledger.
type row struct {
	Archive         string   `json:"archive,omitempty"`
	CallerKeyID     string   `json:"callerKeyId,omitempty"`
	CallerKeyName   string   `json:"callerKeyName,omitempty"`
	Credits         *float64 `json:"credits,omitempty"`
	Time            string   `json:"t"`
	Provider        string   `json:"provider"`
	Agent           string   `json:"agent"`
	Model           string   `json:"model"`
	In              float64  `json:"in"`
	Out             float64  `json:"out"`
	CacheRead       *float64 `json:"cache_read,omitempty"`
	CacheWrite      *float64 `json:"cache_write,omitempty"`
	CacheWrite1h    *float64 `json:"cache_write_1h,omitempty"`
	Reasoning       *float64 `json:"reasoning,omitempty"`
	Ms              float64  `json:"ms"`
	TTFT            float64  `json:"ttft_ms,omitempty"`
	FirstText       float64  `json:"first_text_ms,omitempty"`
	Flow            float64  `json:"flow_ms,omitempty"`
	Status          float64  `json:"status"`
	Speed           float64  `json:"speed,omitempty"`
	Err             string   `json:"err,omitempty"`
	Priced          bool     `json:"priced"`
	Cost            float64  `json:"cost"`
	Requested       string   `json:"req,omitempty"`
	Served          string   `json:"served,omitempty"`
	Endpoint        string   `json:"ep,omitempty"`
	Effort          string   `json:"effort,omitempty"`
	Stop            string   `json:"stop,omitempty"`
	Rejected        bool     `json:"rejected,omitempty"`
	Canceled        bool     `json:"canceled,omitempty"`
	CacheUnknown    bool     `json:"cache_unknown,omitempty"`
	UpBytes         float64  `json:"up_bytes,omitempty"`
	DownBytes       float64  `json:"down_bytes,omitempty"`
	ImageFilesSaved float64  `json:"image_files_saved,omitempty"`
	ImageFiles      float64  `json:"image_files,omitempty"`
}

func (r row) failed() bool { return !r.Canceled && (r.Status >= 400 || r.Err != "") }

func valueOf(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}

// heatDay is one day of the request heatmap.
type heatDay struct {
	Date     string  `json:"date"`
	Calls    float64 `json:"calls"`
	Tokens   float64 `json:"tokens"`
	Cost     float64 `json:"cost"`
	Unpriced float64 `json:"unpriced,omitempty"`
}

// heatmap is the last 53 weeks of the filter's requests, the days with
// any listed.
type heatmap struct {
	From string    `json:"from"`
	To   string    `json:"to"`
	Days []heatDay `json:"days"`
}

// callerKey is a gateway key that made requests in the period.
type callerKey struct {
	ID            string `json:"id"`
	CallerKeyID   string `json:"callerKeyId,omitempty"`
	CallerKeyName string `json:"callerKeyName,omitempty"`
}

// account is a subscription account that answered requests in the period.
type account struct {
	ID       string `json:"id"`
	Account  string `json:"account"`
	Provider string `json:"provider,omitempty"`
	Name     string `json:"name,omitempty"`
}

// ledgerPage is capture.ai.usage.get's answer.
type ledgerPage struct {
	CallerKeys   []callerKey        `json:"callerKeys"`
	Heatmap      *heatmap           `json:"heatmap,omitempty"`
	Rows         []row              `json:"rows"`
	Sum          totals             `json:"sum"`
	Total        int                `json:"total"`
	Offset       int                `json:"offset"`
	By           map[string][]share `json:"by"`
	ChartBy      map[string][]share `json:"chartBy,omitempty"`
	Series       []point            `json:"series"`
	Bucket       string             `json:"bucket,omitempty"`
	Day          string             `json:"day,omitempty"`
	PriceWarning bool               `json:"priceWarning,omitempty"`
	Accounts     []account          `json:"accounts"`
	Purposes     []string           `json:"purposes"`
}

// filter is the ledger query, as React's UsageFilter. The reader's time
// zone (tz) is left out: Magpie reads an absent zone as this computer's
// (usage.Zone("") is time.Local), the zone the window shows times in.
type filter struct {
	CallerKey string `json:"callerKey"`
	Period    string `json:"period"`
	Provider  string `json:"provider"`
	Source    string `json:"source"`
	Failed    bool   `json:"failed"`
	Offset    int    `json:"offset"`
	Day       string `json:"day"`
	Model     string `json:"model"`
	Account   string `json:"account"`
	Purpose   string `json:"purpose"`
	Query     string `json:"query"`
}

// initialFilter is the first period and no filter.
var initialFilter = filter{Period: "7d"}

// narrowed reports whether any filter besides the period and the page is
// set, which Clear filters undoes.
func (f filter) narrowed() bool {
	return f.CallerKey != "" || f.Provider != "" || f.Source != "" || f.Failed || f.Day != "" ||
		f.Model != "" || f.Account != "" || f.Purpose != "" || f.Query != ""
}

// sessionCounts are the sums of registered Chat sessions.
type sessionCounts struct {
	Sessions    float64 `json:"sessions"`
	Messages    float64 `json:"messages"`
	Calls       float64 `json:"calls"`
	Input       float64 `json:"input"`
	Output      float64 `json:"output"`
	CacheRead   float64 `json:"cache_read"`
	CacheWrite  float64 `json:"cache_write"`
	Reasoning   float64 `json:"reasoning"`
	Errors      float64 `json:"errors"`
	Compactions float64 `json:"compactions"`
	ActiveMs    float64 `json:"activeMs"`
	Cost        float64 `json:"cost"`
	Unpriced    float64 `json:"unpriced"`
}

func (s sessionCounts) tokens() float64 { return s.Input + s.Output + s.CacheRead + s.CacheWrite }

// contextPoint is one request's context in a session's history.
type contextPoint struct {
	Time       string  `json:"time"`
	Input      float64 `json:"input"`
	CacheRead  float64 `json:"cache_read"`
	Output     float64 `json:"output"`
	Compaction bool    `json:"compaction,omitempty"`
}

// usageSession is one registered Chat session.
type usageSession struct {
	sessionCounts
	ID        string         `json:"id"`
	Title     string         `json:"title"`
	Project   string         `json:"project"`
	Provider  string         `json:"provider"`
	Model     string         `json:"model"`
	CreatedAt string         `json:"createdAt"`
	UpdatedAt string         `json:"updatedAt"`
	Archived  bool           `json:"archived"`
	Context   []contextPoint `json:"context"`
	Error     string         `json:"error,omitempty"`
	Partial   bool           `json:"partial,omitempty"`
}

// sessionShape is one measure's bins: edges[i] is a bin's least value.
type sessionShape struct {
	Edges  []float64 `json:"edges"`
	Counts []float64 `json:"counts"`
	Total  float64   `json:"total"`
}

// sessionPoint is one day of the sessions.
type sessionPoint struct {
	Time string `json:"time"`
	sessionCounts
}

// sessionReport is chat.usage's answer.
type sessionReport struct {
	Rows     []usageSession                      `json:"rows"`
	Sum      sessionCounts                       `json:"sum"`
	Series   []sessionPoint                      `json:"series"`
	By       map[string]map[string]sessionCounts `json:"by"`
	Total    int                                 `json:"total"`
	Offset   int                                 `json:"offset"`
	Median   float64                             `json:"median"`
	P90      float64                             `json:"p90"`
	Top      []usageSession                      `json:"top"`
	Tools    map[string]float64                  `json:"tools"`
	Skills   map[string]float64                  `json:"skills"`
	Warnings []string                            `json:"warnings"`
	Shape    map[string]sessionShape             `json:"shape"`
}

// sessionQuery is chat.usage's query, beside the period.
type sessionQuery struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
	Project  string `json:"project"`
	Query    string `json:"query"`
	Offset   int    `json:"offset"`
	TopBy    string `json:"topBy"`
}

// catalog is the part of chat.catalog the statistics read: the names of
// projects and conversations.
type catalog struct {
	Projects []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"projects"`
	Conversations []struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"conversations"`
}

// promptItem is one thing in a part of a prompt: a tool, a file, a turn.
type promptItem struct {
	Name   string  `json:"name"`
	Tag    string  `json:"tag,omitempty"`
	N      float64 `json:"n,omitempty"`
	Tokens float64 `json:"tokens"`
}

// promptPart is one kind of what a prompt holds.
type promptPart struct {
	Kind   string       `json:"kind"`
	Tokens float64      `json:"tokens"`
	Items  []promptItem `json:"items,omitempty"`
}

// prompt is Magpie's classified request context.
type prompt struct {
	Window  float64      `json:"window,omitempty"`
	Tokens  float64      `json:"tokens"`
	Counted bool         `json:"counted,omitempty"`
	Held    bool         `json:"held,omitempty"`
	Parts   []promptPart `json:"parts"`
}

// ctxPoint is one request on a session's sparkline.
type ctxPoint struct {
	ID     int64   `json:"id"`
	Time   string  `json:"time"`
	Tokens float64 `json:"tokens"`
}

// ctxSession is one conversation's context over its requests.
type ctxSession struct {
	Agent    string     `json:"agent"`
	Key      string     `json:"key"`
	Title    string     `json:"title,omitempty"`
	Model    string     `json:"model"`
	Requests float64    `json:"requests"`
	Peak     float64    `json:"peak"`
	Window   float64    `json:"window"`
	Compacts float64    `json:"compacts,omitempty"`
	Latest   prompt     `json:"latest"`
	LatestID int64      `json:"latestId"`
	Points   []ctxPoint `json:"points"`
}

// ctxScore is one part of an agent's score.
type ctxScore struct {
	Key    string  `json:"key"`
	Points float64 `json:"points"`
	Most   float64 `json:"most"`
}

// ctxTag is something an agent's prompts tell of it.
type ctxTag struct {
	Key   string   `json:"key"`
	Tone  string   `json:"tone"`
	Value *float64 `json:"value,omitempty"`
}

// ctxAgent is a provider's context habits.
type ctxAgent struct {
	Agent       string             `json:"agent"`
	Requests    float64            `json:"requests"`
	Sessions    float64            `json:"sessions"`
	Median      float64            `json:"median"`
	P90         float64            `json:"p90"`
	Baseline    float64            `json:"baseline"`
	WindowKnown bool               `json:"windowKnown"`
	Cache       float64            `json:"cache"`
	Growth      float64            `json:"growth"`
	Compacts    float64            `json:"compacts"`
	Shares      map[string]float64 `json:"shares"`
	Score       float64            `json:"score"`
	Scores      []ctxScore         `json:"scores"`
	Tags        []ctxTag           `json:"tags"`
}

// ctxRequest is a request whose prompt the page can show.
type ctxRequest struct {
	ID      int64  `json:"id"`
	Session string `json:"session,omitempty"`
	Prompt  prompt `json:"prompt"`
}

// contextReply is the context part of capture.ai.usage.get's answer.
type contextReply struct {
	Context *struct {
		Agents   []ctxAgent   `json:"agents"`
		Sessions []ctxSession `json:"sessions"`
	} `json:"context,omitempty"`
	ContextTotal    int          `json:"contextTotal"`
	ContextRequests []ctxRequest `json:"contextRequests"`
}

// balancePart is one amount of a balance: a wallet, granted credit.
type balancePart struct {
	Label   string   `json:"label,omitempty"`
	Text    string   `json:"text"`
	Percent *float64 `json:"percent,omitempty"`
	Error   string   `json:"error,omitempty"`
}

// quotaWindow is one allowance window of an account: five hours, a week.
type quotaWindow struct {
	Name      string  `json:"name"`
	Used      float64 `json:"used"`
	Unlimited bool    `json:"unlimited,omitempty"`
	ResetsAt  string  `json:"resetsAt,omitempty"`
	Display   string  `json:"display,omitempty"`
	// Family and Pool group windows: the curve leaves a family's out and
	// names a pool's by the pool.
	Family string `json:"family,omitempty"`
	Pool   string `json:"pool,omitempty"`
}

// quotaReading is one recorded reading of a window.
type quotaReading struct {
	At       string  `json:"at"`
	Left     float64 `json:"left"`
	Start    string  `json:"start,omitempty"`
	ResetsAt string  `json:"resetsAt,omitempty"`
}

// quotaLine is one window's recorded readings, oldest first.
type quotaLine struct {
	Name   string         `json:"name"`
	Points []quotaReading `json:"points"`
}

// allowance is one account's or key's reading.
type allowance struct {
	Provider     string           `json:"provider"`
	Name         string           `json:"name"`
	User         string           `json:"user,omitempty"`
	Plan         string           `json:"plan,omitempty"`
	Windows      []quotaWindow    `json:"windows"`
	Balance      string           `json:"balance,omitempty"`
	Estimate     *balanceEstimate `json:"estimate,omitempty"`
	BalanceSplit []balancePart    `json:"balanceSplit,omitempty"`
	BalanceParts []balancePart    `json:"balanceParts,omitempty"`
	Error        string           `json:"error,omitempty"`
	SeeAt        string           `json:"seeAt,omitempty"`
	ReadAt       string           `json:"readAt,omitempty"`
	AsOf         string           `json:"asOf,omitempty"`
	History      []quotaLine      `json:"history,omitempty"`
}

type balanceEstimate struct {
	Set   float64 `json:"set"`
	At    string  `json:"at"`
	Spent float64 `json:"spent"`
}

// archivePart is the request or the reply of an archived request.
type archivePart struct {
	Body      string              `json:"body"`
	Headers   map[string][]string `json:"headers"`
	Truncated bool                `json:"truncated,omitempty"`
	Omitted   bool                `json:"omitted,omitempty"`
}

// archivePreview is capture.ai.archive.read's answer: Magpie's bounded
// preview of a request it archived.
type archivePreview struct {
	Large    bool         `json:"large,omitempty"`
	Request  *archivePart `json:"request,omitempty"`
	Response *archivePart `json:"response,omitempty"`
}
