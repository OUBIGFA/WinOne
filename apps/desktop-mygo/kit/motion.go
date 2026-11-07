package kit

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// MotionStandard reuses Fluent UI's durationNormal / curveDecelerateMin.
// A short, non-overshooting settle is WinOne's default for functional movement;
// MyGo owns interruption and the system ReduceMotion preference. No spring,
// bounce, opacity pulse or delayed first appearance is added.
// Source and original MIT notice: third_party/fluent-motion/ and import-notices.json.
var MotionStandard = ui.Animation{
	Duration: 200 * time.Millisecond,
	Ease:     ui.CubicBezier(0.33, 0, 0.1, 1),
}
