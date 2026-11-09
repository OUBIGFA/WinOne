package aiservices

import (
	"encoding/base64"
	"net/url"
	"strings"

	"github.com/egoist/mygo/ui"
	"winone/desktopmygo/kit"
)

const langfusePath = "/api/public/otel"

func langfuseAddress(value string) string {
	value = strings.TrimSuffix(strings.TrimRight(strings.TrimSpace(value), "/"), langfusePath)
	if value != "" && !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return ""
	}
	return value
}

// These are the upstream Langfuse fields, producing the same OTLP endpoint and
// headers. A different collector's credentials never carry across the switch.
func (a *advancedPanel) langfuseView(c *ui.Context, locked bool) {
	s := &a.settings().OTel
	host, public, secret := a.fields.get("langfuseHost"), a.fields.get("langfusePublic"), a.fields.get("langfuseSecret")
	if host.text == "" {
		host.text = "https://cloud.langfuse.com"
		if strings.HasSuffix(s.Endpoint, langfusePath) {
			host.text = strings.TrimSuffix(s.Endpoint, langfusePath)
		}
	}
	preset := host.text
	if preset != "https://cloud.langfuse.com" && preset != "https://us.cloud.langfuse.com" {
		preset = "other"
	}
	if choiceRow(c, upstreamT("Langfuse host"), "", &preset, options("https://cloud.langfuse.com", upstreamT("Cloud EU"), "https://us.cloud.langfuse.com", upstreamT("Cloud US"), "other", upstreamT("Self-hosted")), locked) && preset != "other" {
		host.text = preset
	}
	if preset == "other" {
		nativeText(c, upstreamT("Langfuse host"), &host.text, locked)
	}
	nativeText(c, upstreamT("Public key"), &public.text, locked)
	ui.Field(c, upstreamT("Secret key"), func() { ui.TextInput(c, &secret.text).FillWidth().Password().Disabled(locked) })
	if kit.Button(c, upstreamT("Save"), nil, false).Disabled(locked || public.text == "" || secret.text == "").Clicked() {
		address := langfuseAddress(host.text)
		if address == "" {
			a.err = upstreamT("Enter your Langfuse address, such as https://langfuse.example.com")
			return
		}
		first := !strings.HasSuffix(s.Endpoint, langfusePath)
		headers := map[string]string{}
		if !first {
			for key, value := range s.Headers {
				if !strings.EqualFold(key, "Authorization") {
					headers[key] = value
				}
			}
		}
		headers["Authorization"] = "Basic " + base64.StdEncoding.EncodeToString([]byte(strings.TrimSpace(public.text)+":"+strings.TrimSpace(secret.text)))
		haveVersion := false
		for key := range headers {
			haveVersion = haveVersion || strings.EqualFold(key, "x-langfuse-ingestion-version")
		}
		if first && !haveVersion {
			headers["x-langfuse-ingestion-version"] = "4"
		}
		s.Endpoint, s.Headers = address+langfusePath, headers
		if first {
			s.Enabled = true
			s.Metrics = false
		}
	}
}
