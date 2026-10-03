package worker

import (
	"errors"
	"fmt"
	"winone/modules/wintray/internal/i18n"
	"winone/modules/wintray/internal/orchestrator"
)

// A completed queue can contain failed entries; only successful/skipped entries permit idle exit.
func startupResultError(language string, results []orchestrator.Result) error {
	var failures []error
	for _, result := range results {
		if !result.Managed {
			failures = append(failures, fmt.Errorf("%s：%s", result.AppName, i18n.TranslateResultMessage(language, result.Message)))
		}
	}
	return errors.Join(failures...)
}
