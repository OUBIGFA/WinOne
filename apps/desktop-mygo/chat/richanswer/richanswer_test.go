package richanswer

import (
	"strings"
	"testing"
)

const sample = `{"version":1,"seq":1,"op":"create","id":"months","kind":"range","props":{"label":"Months","min":1,"max":24,"default":6}}
{"seq":2,"op":"create","id":"cost","kind":"stat","props":{"label":"Total","prefix":"¥","value":{"op":"mul","args":[{"ref":"months"},20]}}}
{"seq":3,"op":"patch","id":"cost","props":{"value":{"op":"mul","args":[{"ref":"months"},18]}}}
`

func TestOperationsBuildAndCalculate(t *testing.T) {
	var s Stream
	doc := s.Update(sample, true)
	if doc.Issues != 0 || len(doc.Roots) != 2 {
		t.Fatalf("doc: %+v", doc)
	}
	values := Resolved(doc, nil)
	if got := Label(doc.Nodes["cost"], values); got != "¥108" {
		t.Fatalf("label: %q", got)
	}
	values = Resolved(doc, map[string]any{"months": float64(2)})
	if got := Label(doc.Nodes["cost"], values); got != "¥36" {
		t.Fatalf("edited: %q", got)
	}
}

func TestStreamingKeepsCommittedContentAndReportsGaps(t *testing.T) {
	var s Stream
	first := `{"version":1,"seq":1,"op":"create","id":"a","kind":"text","props":{"text":"一"}}` + "\n"
	doc := s.Update(first+`{"seq":2,"op":"appe`, false)
	if len(doc.Roots) != 1 || doc.Issues != 0 {
		t.Fatalf("an unfinished line is not an issue: %+v", doc)
	}
	full := first + `{"seq":2,"op":"append","id":"a","text":"二"}` + "\n" + `{"seq":4,"op":"create","id":"b","kind":"text","props":"三"}` + "\n"
	doc = s.Update(full, true)
	if got := String(doc.Nodes["a"], "text", ""); got != "一二" {
		t.Fatalf("append: %q", got)
	}
	if doc.Issues != 1 || String(doc.Nodes["b"], "text", "") != "三" {
		t.Fatalf("the gap is reported and later lines apply: %+v", doc)
	}
	// A corrected snapshot rebuilds.
	doc = s.Update(`{"version":1,"seq":1,"op":"create","id":"z","kind":"divider"}`+"\n", true)
	if len(doc.Nodes) != 1 || doc.Nodes["z"] == nil {
		t.Fatalf("rebuild: %+v", doc)
	}
}

func TestInlineChildrenAndContainerProse(t *testing.T) {
	var s Stream
	source := `{"seq":1,"op":"create","id":"box","kind":"stack","props":{"children":[{"kind":"text","text":"甲"},{"id":"named","kind":"row","children":[{"kind":"divider"}]},{"kind":"nope"}]}}
{"seq":2,"op":"append","id":"box","text":"正文"}
{"seq":3,"op":"append","id":"box","text":"继续"}
{"seq":4,"op":"create","id":"after","kind":"divider","parent":"box"}
{"seq":5,"op":"append","id":"box","text":"末尾"}
`
	doc := s.Update(source, true)
	box := doc.Nodes["box"]
	if len(box.Children) != 5 || doc.Issues != 1 {
		t.Fatalf("children: %v issues %d", box.Children, doc.Issues)
	}
	if doc.Nodes["named"] == nil || len(doc.Nodes["named"].Children) != 1 {
		t.Fatalf("nested inline: %+v", doc.Nodes["named"])
	}
	if got := String(doc.Nodes[box.Children[2]], "text", ""); got != "正文继续" {
		t.Fatalf("prose: %q", got)
	}
	if got := String(doc.Nodes[box.Children[4]], "text", ""); got != "末尾" {
		t.Fatalf("order: %q", got)
	}
}

func TestMoveRejectsCyclesAndRemoveTakesTheSubtree(t *testing.T) {
	var s Stream
	source := `{"seq":1,"op":"create","id":"a","kind":"stack"}
{"seq":2,"op":"create","id":"b","kind":"stack","parent":"a"}
{"seq":3,"op":"move","id":"a","parent":"b"}
{"seq":4,"op":"create","id":"c","kind":"text","props":{"text":"x"},"parent":"b"}
{"seq":5,"op":"move","id":"c","parent":"root"}
{"seq":6,"op":"remove","id":"a"}
`
	doc := s.Update(source, true)
	if doc.Issues != 1 {
		t.Fatalf("cycle must be refused: %d", doc.Issues)
	}
	if doc.Nodes["a"] != nil || doc.Nodes["b"] != nil || len(doc.Roots) != 1 || doc.Roots[0] != "c" {
		t.Fatalf("doc: %+v %v", doc.Nodes, doc.Roots)
	}
}

func TestEvaluateIsWhitelistedAndUnavailableIsNotZero(t *testing.T) {
	values := map[string]any{"n": float64(4), "s": "x", "blank": " "}
	expr := func(op string, args ...any) map[string]any { return map[string]any{"op": op, "args": args} }
	if got := Evaluate(expr("add", map[string]any{"ref": "n"}, float64(1)), values); got != float64(5) {
		t.Fatalf("add: %v", got)
	}
	if got := Evaluate(expr("add", map[string]any{"ref": "missing"}, float64(1)), values); got != nil {
		t.Fatalf("missing must stay unavailable: %v", got)
	}
	if got := Evaluate(expr("add", map[string]any{"ref": "blank"}, float64(1)), values); got != nil {
		t.Fatalf("blank must not become zero: %v", got)
	}
	if got := Evaluate(expr("div", float64(1), float64(0)), values); got != nil {
		t.Fatalf("division by zero: %v", got)
	}
	if got := Evaluate(expr("if", true, "yes", map[string]any{"ref": "missing"}), values); got != "yes" {
		t.Fatalf("only the selected branch runs: %v", got)
	}
	if got := Evaluate(expr("exec", "rm"), values); got != nil {
		t.Fatalf("unknown op: %v", got)
	}
	if got := Evaluate(expr("concat", "a", float64(1), true), values); got != "a1true" {
		t.Fatalf("concat: %v", got)
	}
}

func TestCyclesStayUnavailable(t *testing.T) {
	var s Stream
	doc := s.Update(`{"seq":1,"op":"create","id":"a","kind":"stat","props":{"value":{"ref":"b"}}}
{"seq":2,"op":"create","id":"b","kind":"stat","props":{"value":{"ref":"a"}}}
`, true)
	values := Resolved(doc, nil)
	if !CalculationUnavailable(doc.Nodes["a"], values) {
		t.Fatal("a cycle must be unavailable")
	}
}

func TestSendAndActionsNeedValidValues(t *testing.T) {
	var s Stream
	doc := s.Update(`{"seq":1,"op":"create","id":"go","kind":"button","props":{"label":"Plan","send":{"op":"concat","args":["Plan ",{"ref":"d"}," days"]}}}
{"seq":2,"op":"create","id":"set","kind":"button","props":{"set":{"a":1,"b":{"ref":"missing"}}}}
`, true)
	if got := SendText(doc.Nodes["go"], map[string]any{"d": float64(3)}); got != "Plan 3 days" {
		t.Fatalf("send: %q", got)
	}
	if got := SendText(doc.Nodes["go"], map[string]any{}); got != "" {
		t.Fatalf("send without values: %q", got)
	}
	if got := Action(doc.Nodes["set"], map[string]any{}); len(got) != 0 {
		t.Fatalf("an invalid form must not partly apply: %v", got)
	}
}

func TestLinePlotKeepsPointsInsideAndHidesCrowdedLabels(t *testing.T) {
	var data []Point
	for i := range 40 {
		data = append(data, Point{Label: "第" + strings.Repeat("一", 3), Value: float64(i * i)})
	}
	plot := PlotLine(data, 300, map[string]any{})
	for _, point := range plot.Points {
		if point.X < plot.Left || point.X > plot.Width-plot.Right || point.Y < 0 || point.Y > plot.Height {
			t.Fatalf("point outside: %+v", point)
		}
	}
	if len(plot.Labels) >= len(data) || plot.ShowValues {
		t.Fatalf("crowded labels: %d values %v", len(plot.Labels), plot.ShowValues)
	}
}

func TestVectorAndStyleAcceptOnlyDeclarativeValues(t *testing.T) {
	var s Stream
	doc := s.Update(`{"seq":1,"op":"create","id":"v","kind":"svg","props":{"viewBox":[0,0,10,10],"shapes":[{"type":"path","d":"M0 0L1 1","fill":"danger"},{"type":"path","d":"javascript:alert(1)"},{"type":"script"}],"style":{"color":"#f00","width":"50%","padding":8,"background":"url(x)"}}}
`, true)
	box, shapes := Vector(doc.Nodes["v"])
	if box != [4]float64{0, 0, 10, 10} || len(shapes) != 2 {
		t.Fatalf("vector: %v %+v", box, shapes)
	}
	if _, has := shapes[1].Props["d"]; has {
		t.Fatal("a script path passed")
	}
	style := ReadStyle(doc.Nodes["v"])
	if style.Color != "" || style.Percent["width"] != 50 || style.Lengths["padding"] != 8 {
		t.Fatalf("style: %+v", style)
	}
}
