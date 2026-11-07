package onebot

import (
	"testing"

	"winone/desktopmygo/i18n"
)

func TestChannelsIntroductionUsesTheCurrentCompanionName(t *testing.T) {
	h, p, tt := setup(t, &fakeOneBot{profiles: []map[string]any{oneBot("self", "小柴")}})
	p.settings = newProfileSettings(p, p.profiles[0], "channels")
	defer i18n.SetLocale(i18n.Chinese)
	for _, locale := range []string{i18n.Chinese, i18n.English} {
		i18n.SetLocale(locale)
		for range 4 {
			h.Settle(tt)
		}
		expected := bt("bots.channels.empty", "name", "小柴")
		if !tt.HasText(expected) {
			t.Errorf("%s channel introduction did not substitute its companion: %q", locale, tt.Texts())
		}
		p.settings.draft.Name = "另一个名字"
		h.Settle(tt)
		if !tt.HasText(bt("bots.channels.empty", "name", "另一个名字")) {
			t.Error("the introduction did not follow the edited companion name")
		}
		p.settings.draft.Name = "小柴"
	}
}
