package aiservices

import (
	"encoding/json"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/i18n"
	"winone/desktopmygo/kit"
)

// Keep unfinished structured options with the provider's draft, including
// across provider and tab changes. Invalid JSON blocks Save, never discards
// the last valid value or silently saves it in place of the user's text.
type preferenceField struct { text, shown string; invalid bool }

func (p *page) preferencesInvalid() bool {
	for _, f := range p.preferenceFields { if f.invalid { return true } }
	return false
}

func preferenceJSON[T any](c *ui.Context, p *page, key, label, hint string, value *T, locked bool) {
	if p.preferenceFields == nil { p.preferenceFields = map[string]*preferenceField{} }
	f := p.preferenceFields[key]
	if f == nil {
		b, _ := json.MarshalIndent(value, "", "  ")
		f = &preferenceField{text:string(b),shown:string(b)}; p.preferenceFields[key] = f
	}
	if f.text != f.shown {
		f.shown = f.text
		var next T
		err := json.Unmarshal([]byte(f.text), &next)
		f.invalid = err != nil
		if err == nil { *value = next }
	}
	kit.Stack(c.Key(key), label, hint, func() {
		ui.TextArea(c, &f.text).FillWidth().Lines(2, 8).Font(kit.MonoFont).Label(label).Disabled(locked)
		if f.invalid { kit.Notice(c, kit.Caution, "", i18n.T("ai.optionsJSONInvalid"), nil) }
	})
}

// Use the existing settings controls with the exact upstream fields/units.
// Structured per-account maps and windows remain complete, typed JSON; they
// are not reduced to a fixed subset of account names, days or header names.
func (e *editor) preferences(c *ui.Context, pr *provider, locked bool) {
	s := &pr.ProviderPreferences
	kit.Group(c, nil, i18n.T("ai.providerOptions"), "", func() {
		max := 0.0; if s.MaxAccounts != nil { max = float64(*s.MaxAccounts) }
		numberRow(c, i18n.T("ai.maxAccounts"), &max, 0, 1024, true, "", locked, func(v *float64) { if v==nil {s.MaxAccounts=nil} else {n:=int(*v);s.MaxAccounts=&n} })
		wait := float64(s.Patience)
		numberRow(c, i18n.T("ai.providerPatience"), &wait, -1, 240, true, "", locked, func(v *float64) { s.Patience=int(value(v)) })
		kit.Help(c, i18n.T("ai.providerPatienceHint"))
		kit.Toggle(c, &s.CountFree, i18n.T("ai.countFree"), i18n.T("ai.countFreeHint")).Disabled(locked)
		kit.Toggle(c, &s.CapStop, i18n.T("ai.capStop"), "").Disabled(locked)
		kit.Toggle(c, &s.CapCareful, i18n.T("ai.capCareful"), "").Disabled(locked)
		kit.Toggle(c, &s.AdditionalToolsOff, i18n.T("ai.additionalToolsOff"), "").Disabled(locked)
		kit.Toggle(c, &s.Unredacted, i18n.T("ai.unredacted"), i18n.T("ai.unredactedHint")).Disabled(locked)
		if s.Unredacted && (pr.AuthMode=="account" || !strings.Contains(pr.BaseURL,"127.0.0.1") && !strings.Contains(pr.BaseURL,"localhost")) { kit.Notice(c, kit.Caution,"",i18n.T("ai.unredactedWarning"),nil) }
		if pr.Agent=="antigravity" {kit.Toggle(c,&s.PromptAsWritten,i18n.T("ai.promptAsWritten"),"").Disabled(locked)}
		kit.Toggle(c,&s.CodexAsSent,i18n.T("ai.codexAsSent"),"").Disabled(locked)
		rate := s.PriceRate
		numberRow(c,i18n.T("ai.priceRate"),&rate,0,1000,false,"",locked,func(v *float64){s.PriceRate=value(v)})
		ui.Field(c,i18n.T("ai.providerProxy"),func(){ui.TextInput(c,&s.Proxy).FillWidth().Label(i18n.T("ai.providerProxy")).Disabled(locked)})
		choiceRow(c,i18n.T("ai.authHeader"),"",&s.AuthHeader,options("",i18n.T("ai.automatic"),"x-api-key","x-api-key","bearer","Authorization: Bearer"),locked)
		kit.Toggle(c,&s.InsecureTLS,i18n.T("ai.insecureTLS"),i18n.T("ai.insecureTLSHint")).Disabled(locked)
		if pr.AuthMode=="api-key" {ui.Field(c,i18n.T("ai.keyPlan"),func(){ui.TextInput(c,&s.KeyPlan).FillWidth().Label(i18n.T("ai.keyPlan")).Disabled(locked)})}
		jsonField := func(key string) string { return pr.ID+":"+key }
		preferenceJSON(c,e.p,jsonField("passHeaders"),i18n.T("ai.passHeaders"),i18n.T("ai.passHeadersHint"),&s.PassHeaders,locked)
		preferenceJSON(c,e.p,jsonField("replyHeaders"),i18n.T("ai.replyHeaders"),"",&s.ReplyHeaders,locked)
		preferenceJSON(c,e.p,jsonField("extraBody"),i18n.T("ai.extraBody"),i18n.T("ai.extraBodyHint"),&s.ExtraBody,locked)
		preferenceJSON(c,e.p,jsonField("rateWindows"),i18n.T("ai.rateWindows"),i18n.T("ai.rateWindowsHint"),&s.RateWindows,locked)
		preferenceJSON(c,e.p,jsonField("accountWeights"),i18n.T("ai.accountWeights"),i18n.T("ai.accountMapHint"),&s.AccountWeights,locked)
		preferenceJSON(c,e.p,jsonField("accountConcurrency"),i18n.T("ai.accountConcurrency"),i18n.T("ai.accountMapHint"),&s.AccountConcurrency,locked)
		preferenceJSON(c,e.p,jsonField("accountModels"),i18n.T("ai.accountModels"),i18n.T("ai.accountMapHint"),&s.AccountModels,locked)
		preferenceJSON(c,e.p,jsonField("accountProxies"),i18n.T("ai.accountProxies"),i18n.T("ai.accountMapHint"),&s.AccountProxies,locked)
		preferenceJSON(c,e.p,jsonField("accountNames"),i18n.T("ai.accountNames"),i18n.T("ai.accountMapHint"),&s.AccountNames,locked)
		preferenceJSON(c,e.p,jsonField("accountCaps"),i18n.T("ai.accountCaps"),i18n.T("ai.accountMapHint"),&s.AccountCaps,locked)
		preferenceJSON(c,e.p,jsonField("accountWindowCaps"),i18n.T("ai.accountWindowCaps"),i18n.T("ai.windowCapsHint"),&s.AccountWindowCaps,locked)
		preferenceJSON(c,e.p,jsonField("keySpendCaps"),i18n.T("ai.keySpendCaps"),i18n.T("ai.keySpendCapsHint"),&s.KeySpendCaps,locked)
		preferenceJSON(c,e.p,jsonField("autoGroupSkip"),i18n.T("ai.autoGroupSkip"),"",&s.AutoGroupSkip,locked)
	})
}
