package ai

import (
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"strings"
	"time"
)

// ProviderPreferences mirrors the writable Magpie provider options used by
// WinOne. The same DTO is copied into its AI worker by build/build-ai.ps1;
// secrets and discovered accounts are not part of it.
// Zero/absent values keep upstream defaults; original identifiers and units
// stay intact rather than introducing a second routing or pricing policy.
type ProviderPreferences struct {
	Contexts map[string]int `json:"contexts,omitempty"`
	Website  string         `json:"website,omitempty"`
	KeysURL  string         `json:"keysUrl,omitempty"`
	Off      bool           `json:"off,omitempty"`
	// Write-only edits on the public configuration. The Store seals them
	// separately and hydrates only private execution snapshots.
	AccessKeyID          *string                       `json:"accessKeyID,omitempty"`
	SecretAccessKey      *string                       `json:"secretAccessKey,omitempty"`
	BalanceToken         *string                       `json:"balanceToken,omitempty"`
	HasAccessKeyID       bool                          `json:"hasAccessKeyID,omitempty"`
	HasSecretAccessKey   bool                          `json:"hasSecretAccessKey,omitempty"`
	HasBalanceToken      bool                          `json:"hasBalanceToken,omitempty"`
	Endpoints            *ProviderEndpoints            `json:"endpoints,omitempty"`
	Fallback             []string                      `json:"fallback,omitempty"`
	QuotaRest            int                           `json:"quotaRest,omitempty"`
	QuotaResetAt         string                        `json:"quotaResetAt,omitempty"`
	RestCap              int                           `json:"restCap,omitempty"`
	NativeCompaction     bool                          `json:"nativeCompaction,omitempty"`
	PinUpstream          bool                          `json:"pinUpstream,omitempty"`
	ModelsURL            string                        `json:"modelsURL,omitempty"`
	ListOff              bool                          `json:"listOff,omitempty"`
	RefreshEvery         string                        `json:"refreshEvery,omitempty"`
	AutoInclude          []string                      `json:"autoInclude,omitempty"`
	Unlisted             bool                          `json:"unlisted,omitempty"`
	HiddenDrawers        []string                      `json:"hiddenDrawers,omitempty"`
	HiddenVideomakers    []string                      `json:"hiddenVideomakers,omitempty"`
	BalanceURL           string                        `json:"balanceURL,omitempty"`
	BalancePath          string                        `json:"balancePath,omitempty"`
	BalanceMethod        string                        `json:"balanceMethod,omitempty"`
	ZhipuTeam            *ZhipuTeam                    `json:"zhipuTeam,omitempty"`
	Vertex               *Vertex                       `json:"vertex,omitempty"`
	Family               string                        `json:"family,omitempty"`
	Catalog              string                        `json:"catalog,omitempty"`
	KeyPlan              string                        `json:"keyPlan,omitempty"`
	MaxAccounts          *int                          `json:"maxAccounts,omitempty"`
	AccountConcurrency   map[string]int                `json:"accountConcurrency,omitempty"`
	AccountWeights       map[string]int                `json:"accountWeights,omitempty"`
	AccountModels        map[string][]string           `json:"accountModels,omitempty"`
	AccountProxies       map[string]string             `json:"accountProxies,omitempty"`
	AccountCaps          map[string]int                `json:"accountCaps,omitempty"`
	AccountWindowCaps    map[string]map[string]int     `json:"accountWindowCaps,omitempty"`
	AccountWindowAmounts map[string]map[string]float64 `json:"accountWindowAmounts,omitempty"`
	AccountNames         map[string]string             `json:"accountNames,omitempty"`
	KeySpendCaps         map[string]SpendCap           `json:"keySpendCaps,omitempty"`
	KeyOwnBalances       map[string]OwnBalance         `json:"keyOwnBalances,omitempty"`
	KeyPriceRates        map[string]float64            `json:"keyPriceRates,omitempty"`
	MagpieUserAgent      bool                          `json:"magpieUserAgent,omitempty"`
	PriceRate            float64                       `json:"priceRate,omitempty"`
	CountFree            bool                          `json:"countFree,omitempty"`
	RateWindows          []RateWindow                  `json:"rateWindows,omitempty"`
	ReplyHeaders         []string                      `json:"replyHeaders,omitempty"`
	PassHeaders          []string                      `json:"passHeaders,omitempty"`
	ExtraBody            map[string]any                `json:"extraBody,omitempty"`
	PromptAsWritten      bool                          `json:"promptAsWritten,omitempty"`
	AdditionalToolsOff   bool                          `json:"additionalToolsOff,omitempty"`
	Patience             int                           `json:"patience,omitempty"`
	CapStop              bool                          `json:"capStop,omitempty"`
	CapCareful           bool                          `json:"capCareful,omitempty"`
	BestInFamily         bool                          `json:"bestInFamily,omitempty"`
	Proxy                string                        `json:"proxy,omitempty"`
	AuthHeader           string                        `json:"authHeader,omitempty"`
	CodexAsSent          bool                          `json:"codexAsSent,omitempty"`
	InsecureTLS          bool                          `json:"insecureTLS,omitempty"`
	HTTP1                bool                          `json:"http1,omitempty"`
	ImageFilesOff        bool                          `json:"imageFilesOff,omitempty"`
	PassErrors           bool                          `json:"passErrors,omitempty"`
	UseFirst             bool                          `json:"useFirst,omitempty"`
	KeepSelf             bool                          `json:"keepSelf,omitempty"`
	NoStandIn            bool                          `json:"noStandIn,omitempty"`
	UpstreamProviders    bool                          `json:"upstreamProviders,omitempty"`
	NoBalanceProbe       bool                          `json:"noBalanceProbe,omitempty"`
	Socket               string                        `json:"socket,omitempty"`
	// Certificate settings store user-selected file paths, never PEM contents.
	ClientCert       string              `json:"clientCert,omitempty"`
	ClientKey        string              `json:"clientKey,omitempty"`
	ServerCA         string              `json:"serverCA,omitempty"`
	ServerPin        string              `json:"serverPin,omitempty"`
	Resolve          map[string][]string `json:"resolve,omitempty"`
	AccountModelsAll map[string]bool     `json:"accountModelsAll,omitempty"`
}

// Nil keeps the preset's native endpoints. An explicit set replaces them;
// Protocol/BaseURL still identify and edit the primary endpoint for older clients.
type ProviderEndpoints struct {
	Chat      string `json:"chat,omitempty"`
	Responses string `json:"responses,omitempty"`
	Anthropic string `json:"anthropic,omitempty"`
	Gemini    string `json:"gemini,omitempty"`
	Decide    string `json:"decide,omitempty"`
}

// These options travel with a particular credential through each retry and
// route. They never change the credential's stable Host ID.
type KeyOptions struct {
	Name     string `json:"name,omitempty"`
	Protocol string `json:"protocol,omitempty"`
	Weight   int    `json:"weight,omitempty"`
	Plan     string `json:"plan,omitempty"`
}

type ZhipuTeam struct {
	Org     string `json:"org,omitempty"`
	Project string `json:"project,omitempty"`
}

// Credentials is a user-selected file path, never credential file contents.
type Vertex struct {
	Project     string `json:"project,omitempty"`
	Location    string `json:"location,omitempty"`
	Credentials string `json:"credentials,omitempty"`
	Impersonate string `json:"impersonate,omitempty"`
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

// Magpie subtracts spending after At, converting its USD ledger for CNY.
type OwnBalance struct {
	USD    float64        `json:"usd,omitempty"`
	CNY    *float64       `json:"cny,omitempty"`
	At     time.Time      `json:"at"`
	Refill *BalanceRefill `json:"refill,omitempty"`
}

type BalanceRefill struct {
	Every string `json:"every"`
	Day   int    `json:"day"`
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
	if len(p.Contexts) > 4096 {
		return errors.New("上下文规则过多")
	}
	for id, size := range p.Contexts {
		if id == "" || len(id) > 256 || size < 0 || size > 100000000 {
			return errors.New("上下文规则无效")
		}
	}
	for _, address := range []string{p.Website, p.KeysURL} {
		if address != "" {
			u, err := url.Parse(address)
			if err != nil || u.Hostname() == "" || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") {
				return errors.New("供应商网站地址无效")
			}
		}
	}
	if _, err := ParseRefreshEvery(p.RefreshEvery); err != nil {
		return err
	}
	if _, err := ParseResetAt(p.QuotaResetAt); err != nil {
		return err
	}
	if _, err := CleanAutoInclude(p.AutoInclude); err != nil {
		return err
	}
	addresses := []string{p.ModelsURL, p.BalanceURL}
	if e := p.Endpoints; e != nil {
		addresses = append(addresses, e.Chat, e.Responses, e.Anthropic, e.Gemini, e.Decide)
	}
	for _, address := range addresses {
		if address == "" {
			continue
		}
		u, err := url.Parse(address)
		if err != nil || len(address) > 2048 || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Scheme != "https" && u.Scheme != "http" {
			return errors.New("供应商地址需要有效的 HTTP 或 HTTPS URL")
		}
	}
	if p.QuotaRest < 0 || p.RestCap < -1 {
		return errors.New("供应商休息时间无效")
	}
	if p.BalanceMethod != "" && p.BalanceMethod != "GET" && p.BalanceMethod != "POST" {
		return errors.New("余额请求方式须为 GET 或 POST")
	}
	if p.MaxAccounts != nil && (*p.MaxAccounts < 0 || *p.MaxAccounts > 1024) || p.Patience < -1 || p.Patience > 240 {
		return errors.New("账号数量或请求耐心时间无效")
	}
	if math.IsNaN(p.PriceRate) || math.IsInf(p.PriceRate, 0) || p.PriceRate < 0 || p.PriceRate > 1000 || len(p.RateWindows) > 8 {
		return errors.New("价格倍率或计费时间段无效")
	}
	if p.Socket != "" && p.Socket != "auto" && p.Socket != "on" {
		return errors.New("Responses WebSocket 模式无效")
	}
	// Only bound document input here; upstream checks the certificate and
	// connection rules when it applies the selected provider to a request.
	for _, path := range []string{p.ClientCert, p.ClientKey, p.ServerCA} {
		if len(path) > 4096 || strings.ContainsRune(path, '\x00') {
			return errors.New("证书文件路径无效")
		}
	}
	for _, headers := range [][]string{p.PassHeaders, p.ReplyHeaders} {
		if len(headers) > 128 {
			return errors.New("请求或回复头过多")
		}
		for _, name := range headers {
			if name == "" || len(name) > 256 || strings.ContainsAny(name, "\r\n\x00:") {
				return errors.New("请求或回复头名称无效")
			}
		}
	}
	if p.AuthHeader != "" && p.AuthHeader != "x-api-key" && p.AuthHeader != "bearer" {
		return errors.New("认证请求头模式无效")
	}
	if err := validateMagpieProxy(p.Proxy); err != nil {
		return errors.New("供应商代理地址无效")
	}
	for _, proxy := range p.AccountProxies {
		if err := validateMagpieProxy(proxy); err != nil {
			return errors.New("账号代理地址无效")
		}
	}
	for _, caps := range p.AccountWindowCaps {
		for _, v := range caps {
			if v < 1 || v > 100 {
				return errors.New("账号窗口上限须为 1–100%")
			}
		}
	}
	for user, amounts := range p.AccountWindowAmounts {
		for window, amount := range amounts {
			if math.IsNaN(amount) || math.IsInf(amount, 0) || amount <= 0 {
				return errors.New("账号窗口固定上限须为正数")
			}
			if _, hasShare := p.AccountWindowCaps[user][window]; hasShare {
				return errors.New("同一账号窗口只能选择百分比或固定上限")
			}
		}
	}
	for _, rate := range p.KeyPriceRates {
		if math.IsNaN(rate) || math.IsInf(rate, 0) || rate <= 0 || rate > 1000 || math.Abs(rate*1000-math.Round(rate*1000)) > 1e-6 {
			return errors.New("密钥价格倍率须大于 0、不超过 1000，最多三位小数")
		}
	}
	for _, v := range p.AccountCaps {
		if v < 1 || v > 99 {
			return errors.New("账号额度上限须为 1–99%")
		}
	}
	for _, v := range p.AccountWeights {
		if v < 1 || v > 1000 {
			return errors.New("账号权重须为 1–1000")
		}
	}
	for _, v := range p.AccountConcurrency {
		if v < 0 || v > 1024 {
			return errors.New("账号并发上限无效")
		}
	}
	for _, cap := range p.KeySpendCaps {
		if math.IsNaN(cap.USD) || math.IsInf(cap.USD, 0) || cap.USD <= 0 || cap.USD > 1000000 || cap.Period != "day" && cap.Period != "month" && cap.Period != "total" {
			return errors.New("密钥消费上限无效")
		}
	}
	for _, balance := range p.KeyOwnBalances {
		amount := balance.USD
		if balance.CNY != nil {
			amount = *balance.CNY
		}
		if math.IsNaN(amount) || math.IsInf(amount, 0) || amount < 0 || amount > 1000000 || balance.At.IsZero() {
			return errors.New("手动余额需要有效金额及记录时间")
		}
		if r := balance.Refill; r != nil {
			if r.Every != "week" && r.Every != "month" || r.Day < 1 || r.Every == "week" && r.Day > 7 || r.Every == "month" && r.Day > 31 {
				return errors.New("余额重置需要有效的周期及日期")
			}
		}
	}
	clock := func(s string, end bool) (time.Time, bool) {
		if end && s == "24:00" {
			return time.Date(0, 1, 2, 0, 0, 0, 0, time.UTC), true
		}
		t, err := time.Parse("15:04", s)
		return t, err == nil
	}
	for _, window := range p.RateWindows {
		start, a := clock(window.Start, false)
		end, b := clock(window.End, true)
		if !a || !b || start.Equal(end) || !(window.Rate > 0) || window.Rate > 1000 || math.IsInf(window.Rate, 0) || math.Abs(window.Rate*1000-math.Round(window.Rate*1000)) > 1e-6 {
			return errors.New("计费时间段需要不同的起止时间和正数倍率（最多三位小数）")
		}
		for _, d := range window.Days {
			if d < 0 || d > 6 {
				return errors.New("计费星期须为 0（周日）至 6（周六）")
			}
		}
	}
	data, err := json.Marshal(p)
	if err != nil || len(data) > 256<<10 {
		return errors.New("供应商高级选项过大或无效")
	}
	return nil
}
