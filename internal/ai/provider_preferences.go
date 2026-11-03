package ai

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"
)

// ProviderPreferences mirrors the writable Magpie provider options used by
// WinOne. The same DTO is copied into its AI worker by build/build-ai.ps1;
// secrets, endpoints, discovered accounts and model lists are not part of it.
// Zero/absent values keep upstream defaults; original identifiers and units
// stay intact rather than introducing a second routing or pricing policy.
type ProviderPreferences struct {
	KeyPlan             string                    `json:"keyPlan,omitempty"`
	MaxAccounts         *int                      `json:"maxAccounts,omitempty"`
	AccountConcurrency  map[string]int            `json:"accountConcurrency,omitempty"`
	AccountWeights      map[string]int            `json:"accountWeights,omitempty"`
	AccountModels       map[string][]string       `json:"accountModels,omitempty"`
	AccountProxies      map[string]string         `json:"accountProxies,omitempty"`
	AccountCaps         map[string]int            `json:"accountCaps,omitempty"`
	AccountWindowCaps   map[string]map[string]int `json:"accountWindowCaps,omitempty"`
	AccountNames        map[string]string         `json:"accountNames,omitempty"`
	KeySpendCaps        map[string]SpendCap       `json:"keySpendCaps,omitempty"`
	PriceRate           float64                   `json:"priceRate,omitempty"`
	CountFree           bool                      `json:"countFree,omitempty"`
	RateWindows         []RateWindow              `json:"rateWindows,omitempty"`
	ReplyHeaders        []string                  `json:"replyHeaders,omitempty"`
	PassHeaders         []string                  `json:"passHeaders,omitempty"`
	ExtraBody           map[string]any            `json:"extraBody,omitempty"`
	PromptAsWritten     bool                      `json:"promptAsWritten,omitempty"`
	AdditionalToolsOff  bool                      `json:"additionalToolsOff,omitempty"`
	Patience            int                       `json:"patience,omitempty"`
	Unredacted          bool                      `json:"unredacted,omitempty"`
	CapStop             bool                      `json:"capStop,omitempty"`
	CapCareful          bool                      `json:"capCareful,omitempty"`
	AutoGroupSkip       []string                  `json:"autoGroupSkip,omitempty"`
	BestInFamily        bool                      `json:"bestInFamily,omitempty"`
	Proxy               string                    `json:"proxy,omitempty"`
	AuthHeader          string                    `json:"authHeader,omitempty"`
	CodexAsSent         bool                      `json:"codexAsSent,omitempty"`
	InsecureTLS         bool                      `json:"insecureTLS,omitempty"`
}

// RateWindow is upstream's local-clock window (Sunday=0). The first match
// scales the provider's rate; an end of 24:00 and overnight windows are valid.
type RateWindow struct {
	Days  []int   `json:"days,omitempty"`
	Start string  `json:"start"`
	End   string  `json:"end"`
	Rate  float64 `json:"rate"`
}

type SpendCap struct {
	USD    float64   `json:"usd"`
	Period string    `json:"period"`
	Since  time.Time `json:"since,omitzero"`
}

func (p ProviderPreferences) clone() ProviderPreferences {
	data, _ := json.Marshal(p)
	var result ProviderPreferences
	_ = json.Unmarshal(data, &result)
	return result
}

// Host validation bounds the saved document. Provider-specific normalization
// and execution remain Magpie's; model reasoning words are never checked here.
func (p ProviderPreferences) validate() error {
	if p.MaxAccounts != nil && (*p.MaxAccounts < 0 || *p.MaxAccounts > 1024) || p.Patience < -1 || p.Patience > 240 {
		return errors.New("账号数量或请求耐心时间无效")
	}
	if math.IsNaN(p.PriceRate) || math.IsInf(p.PriceRate, 0) || p.PriceRate < 0 || p.PriceRate > 1000 || len(p.RateWindows) > 8 {
		return errors.New("价格倍率或计费时间段无效")
	}
	for _, headers := range [][]string{p.PassHeaders, p.ReplyHeaders} {
		if len(headers) > 128 { return errors.New("请求或回复头过多") }
		for _, name := range headers {
			if name == "" || len(name) > 256 || strings.ContainsAny(name, "\r\n\x00:") { return errors.New("请求或回复头名称无效") }
		}
	}
	if p.AuthHeader != "" && p.AuthHeader != "x-api-key" && p.AuthHeader != "bearer" { return errors.New("认证请求头模式无效") }
	if err := validateMagpieProxy(p.Proxy); err != nil { return errors.New("供应商代理地址无效") }
	for _, proxy := range p.AccountProxies {
		if err := validateMagpieProxy(proxy); err != nil { return errors.New("账号代理地址无效") }
	}
	for _, caps := range p.AccountWindowCaps {
		for _, v := range caps { if v < 1 || v > 100 { return errors.New("账号窗口上限须为 1–100%") } }
	}
	for _, v := range p.AccountCaps { if v < 1 || v > 99 { return errors.New("账号额度上限须为 1–99%") } }
	for _, v := range p.AccountWeights { if v < 1 || v > 1000 { return errors.New("账号权重须为 1–1000") } }
	for _, v := range p.AccountConcurrency { if v < 0 || v > 1024 { return errors.New("账号并发上限无效") } }
	for _, cap := range p.KeySpendCaps {
		if math.IsNaN(cap.USD) || math.IsInf(cap.USD, 0) || cap.USD <= 0 || cap.USD > 1000000 || cap.Period != "day" && cap.Period != "month" && cap.Period != "total" { return errors.New("密钥消费上限无效") }
	}
	clock := func(s string, end bool) (time.Time, bool) {
		if end && s == "24:00" { return time.Date(0,1,2,0,0,0,0,time.UTC), true }
		t, err := time.Parse("15:04", s); return t, err == nil
	}
	for _, window := range p.RateWindows {
		start, a := clock(window.Start, false); end, b := clock(window.End, true)
		if !a || !b || start.Equal(end) || !(window.Rate > 0) || window.Rate > 1000 || math.IsInf(window.Rate,0) || math.Abs(window.Rate*1000-math.Round(window.Rate*1000)) > 1e-6 { return errors.New("计费时间段需要不同的起止时间和正数倍率（最多三位小数）") }
		for _, d := range window.Days { if d < 0 || d > 6 { return errors.New("计费星期须为 0（周日）至 6（周六）") } }
	}
	data, err := json.Marshal(p)
	if err != nil || len(data) > 256<<10 { return errors.New("供应商高级选项过大或无效") }
	return nil
}
