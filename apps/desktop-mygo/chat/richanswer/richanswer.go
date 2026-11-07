// Package richanswer is a native port of Cindy's declarative answer components
// (packages/maker-shared/src/richAnswer.ts and richAnswerVisual.ts): the fenced
// cindy-ui operations a reply can carry, parsed into a tree. It evaluates only
// whitelisted arithmetic: no code runs and nothing here does any I/O. Tracked as
// chat-rich-answer in third_party/cindy/native-ports.json.
package richanswer

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Language is the fence's language.
const Language = "cindy-ui"

const (
	maxSource = 2_000_000
	maxNodes  = 2048
	maxDepth  = 32
	maxProps  = 65_536
)

var (
	kinds = []string{"text", "markdown", "heading", "icon", "stack", "row", "grid", "group", "divider",
		"image", "video", "audio", "code", "math", "diagram", "stat", "chart", "svg",
		"range", "input", "check", "select", "button", "form", "details", "tabs"}
	containers = []string{"stack", "row", "grid", "group", "form", "details", "tabs"}
	textKinds  = []string{"text", "markdown", "heading", "code", "math", "diagram"}
	idPattern  = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
)

// Node is one component.
type Node struct {
	ID, Kind string
	Props    map[string]any
	Children []string
}

// IsContainer reports a kind whose children are separate nodes.
func IsContainer(kind string) bool { return slices.Contains(containers, kind) }

// Document is the tree an answer's operations have built so far.
type Document struct {
	Nodes  map[string]*Node
	Roots  []string
	Issues int
}

// Stream applies one fragment's operations. Appended source parses once; any
// correction to what was read already rebuilds the fragment.
type Stream struct {
	source       string
	offset, seq  int
	doc          Document
	implicit     map[string]string
	failedFinal  *checkpoint
	sourceTooBig bool
}

type checkpoint struct{ offset, seq, issues int }

// Update reads source; complete says its last line is whole though it has no newline.
func (s *Stream) Update(source string, complete bool) *Document {
	switch {
	case s.doc.Nodes == nil || !strings.HasPrefix(source, s.source):
		*s = Stream{doc: Document{Nodes: map[string]*Node{}}, implicit: map[string]string{}}
	case s.failedFinal != nil && len(source) > len(s.source) && len(source) <= maxSource:
		// Retry only the failed line: earlier content can differ from reparsing it
		// after malformed suffixes were already consumed.
		s.offset, s.seq, s.doc.Issues = s.failedFinal.offset, s.failedFinal.seq, s.failedFinal.issues
		s.failedFinal = nil
	}
	if len(source) > maxSource {
		s.sourceTooBig = true
		return s.snapshot()
	}
	s.sourceTooBig = false
	s.source = source
	for {
		end := strings.IndexByte(source[s.offset:], '\n')
		if end < 0 {
			break
		}
		s.line(source[s.offset : s.offset+end])
		s.failedFinal = nil
		s.offset += end + 1
	}
	if complete && s.offset < len(source) {
		point := checkpoint{s.offset, s.seq, s.doc.Issues}
		failed := !s.line(source[s.offset:])
		s.failedFinal = nil
		// A suffix after a committed tail is not a whole line to retry.
		if failed && (s.offset == 0 || source[s.offset-1] == '\n') {
			s.failedFinal = &point
		}
		s.offset = len(source)
	}
	return s.snapshot()
}

func (s *Stream) snapshot() *Document {
	doc := s.doc
	if s.sourceTooBig {
		doc.Issues = max(1, doc.Issues)
	}
	return &doc
}

func (s *Stream) line(line string) (ok bool) {
	if strings.TrimSpace(line) == "" {
		return true
	}
	defer func() {
		if !ok {
			s.doc.Issues++
		}
	}()
	var op map[string]any
	if json.Unmarshal([]byte(line), &op) != nil {
		return false
	}
	seqValue, isNumber := op["seq"].(float64)
	if !isNumber || seqValue < 1 || seqValue != math.Trunc(seqValue) {
		return false
	}
	seq := int(seqValue)
	if seq <= s.seq {
		return true
	}
	// A malformed or absent line leaves a gap: committed content stays, the
	// missing operations are reported, and later lines still apply.
	s.doc.Issues += min(seq-s.seq-1, maxNodes)
	s.seq = seq
	if version, has := op["version"]; has && version != float64(1) {
		return false
	}
	id, _ := op["id"].(string)
	if !idPattern.MatchString(id) {
		return false
	}
	current := s.doc.Nodes[id]
	kind, _ := op["kind"].(string)
	props, err := readProps(op["props"], kind)
	if err != nil {
		return false
	}
	// Some outputs place the visual style beside props.
	if style, isMap := op["style"].(map[string]any); isMap && props["style"] == nil {
		props["style"] = style
	}
	switch op["op"] {
	case "create":
		return s.create(id, kind, props, op, current)
	case "append":
		return s.appendText(current, op)
	case "patch":
		if current == nil {
			return false
		}
		next := *current
		next.Props = map[string]any{}
		for key, value := range current.Props {
			next.Props[key] = value
		}
		for key, value := range props {
			next.Props[key] = value
		}
		s.doc.Nodes[id] = &next
		return true
	case "remove":
		if current == nil {
			return false
		}
		s.remove(id)
		return true
	case "move":
		return s.move(current, op)
	}
	return false
}

func (s *Stream) create(id, kind string, props map[string]any, op map[string]any, current *Node) bool {
	if current != nil || len(s.doc.Nodes) >= maxNodes || !slices.Contains(kinds, kind) {
		return false
	}
	parentID, hasParent := op["parent"]
	var parent *Node
	if hasParent && parentID != "root" {
		name, _ := parentID.(string)
		parent = s.doc.Nodes[name]
		if parent == nil || !IsContainer(parent.Kind) || s.depth(parent.ID) >= maxDepth {
			return false
		}
	}
	node := &Node{ID: id, Kind: kind, Props: props}
	s.doc.Nodes[id] = node
	s.insert(parent, id, op["index"])
	if inline, isList := props["children"].([]any); isList && IsContainer(kind) {
		delete(props, "children")
		s.attachInline(id, inline, s.depth(id)+1)
	}
	return true
}

func (s *Stream) insert(parent *Node, id string, index any) {
	list := s.doc.Roots
	if parent != nil {
		list = parent.Children
	}
	at := len(list)
	if value, isNumber := index.(float64); isNumber && value == math.Trunc(value) {
		at = min(max(int(value), 0), len(list))
	}
	next := slices.Insert(slices.Clone(list), at, id)
	if parent != nil {
		parent.Children = next
	} else {
		s.doc.Roots = next
	}
}

func (s *Stream) appendText(current *Node, op map[string]any) bool {
	text, isText := op["text"].(string)
	if current == nil || !isText {
		return false
	}
	if slices.Contains(textKinds, current.Kind) {
		current.Props["text"] = String(current, "text", "") + text
		return true
	}
	if !IsContainer(current.Kind) {
		return false
	}
	// Models append prose straight to containers. It lands in one implicit
	// markdown child, reused only while it is still the last child, so the answer
	// keeps the order the model wrote it in.
	existing := s.implicit[current.ID]
	if child := s.doc.Nodes[existing]; child != nil && child.Kind == "markdown" && len(current.Children) > 0 && current.Children[len(current.Children)-1] == existing {
		child.Props["text"] = String(child, "text", "") + text
		return true
	}
	if len(s.doc.Nodes) >= maxNodes || s.depth(current.ID) >= maxDepth {
		return false
	}
	id := s.inlineID(current.ID, 1)
	s.doc.Nodes[id] = &Node{ID: id, Kind: "markdown", Props: map[string]any{"text": text}}
	current.Children = append(slices.Clone(current.Children), id)
	s.implicit[current.ID] = id
	return true
}

func (s *Stream) remove(id string) {
	var drop func(string)
	drop = func(name string) {
		if node := s.doc.Nodes[name]; node != nil {
			for _, child := range node.Children {
				drop(child)
			}
		}
		delete(s.doc.Nodes, name)
		delete(s.implicit, name)
	}
	drop(id)
	for container, child := range s.implicit {
		if s.doc.Nodes[child] == nil {
			delete(s.implicit, container)
		}
	}
	s.detach(id)
}

func (s *Stream) detach(id string) {
	s.doc.Roots = slices.DeleteFunc(slices.Clone(s.doc.Roots), func(x string) bool { return x == id })
	for _, node := range s.doc.Nodes {
		if slices.Contains(node.Children, id) {
			node.Children = slices.DeleteFunc(slices.Clone(node.Children), func(x string) bool { return x == id })
		}
	}
}

func (s *Stream) move(current *Node, op map[string]any) bool {
	if current == nil {
		return false
	}
	var parent *Node
	if op["parent"] != "root" {
		name, _ := op["parent"].(string)
		parent = s.doc.Nodes[name]
		if parent == nil || !IsContainer(parent.Kind) || s.descendant(current.ID, parent.ID) {
			return false
		}
	}
	var height func(string) int
	height = func(id string) int {
		tallest := 0
		if node := s.doc.Nodes[id]; node != nil {
			for _, child := range node.Children {
				tallest = max(tallest, height(child))
			}
		}
		return 1 + tallest
	}
	base := 0
	if parent != nil {
		base = s.depth(parent.ID)
	}
	if base+height(current.ID) > maxDepth {
		return false
	}
	s.detach(current.ID)
	s.insert(parent, current.ID, op["index"])
	return true
}

// attachInline makes children written inline in props the nodes they were meant
// to be. One unreadable draft never takes its siblings down with it.
func (s *Stream) attachInline(parentID string, drafts []any, depth int) {
	parent := s.doc.Nodes[parentID]
	if parent == nil {
		return
	}
	for offset, raw := range drafts {
		draft, isMap := raw.(map[string]any)
		kind, _ := draft["kind"].(string)
		if !isMap || !slices.Contains(kinds, kind) || depth > maxDepth || len(s.doc.Nodes) >= maxNodes {
			s.doc.Issues++
			continue
		}
		source := draft["props"]
		if text, isText := draft["text"].(string); source == nil && isText {
			source = map[string]any{"text": text}
		}
		props, err := readProps(source, kind)
		if err != nil {
			s.doc.Issues++
			continue
		}
		// A declared id wins while it is free: tabs' default may point at it.
		id, _ := draft["id"].(string)
		if !idPattern.MatchString(id) || s.doc.Nodes[id] != nil {
			id = s.inlineID(parentID, offset+1)
		}
		s.doc.Nodes[id] = &Node{ID: id, Kind: kind, Props: props}
		parent.Children = append(slices.Clone(parent.Children), id)
		nested, isList := draft["children"].([]any)
		if !isList {
			nested, isList = props["children"].([]any)
		}
		if isList && IsContainer(kind) {
			delete(props, "children")
			s.attachInline(id, nested, depth+1)
		}
	}
}

func (s *Stream) inlineID(parentID string, ordinal int) string {
	for attempt := 1; ; attempt++ {
		suffix := fmt.Sprintf("-%d", ordinal)
		if attempt > 1 {
			suffix = fmt.Sprintf("-%d-%d", ordinal, attempt)
		}
		id := parentID[:min(len(parentID), 80-len(suffix))] + suffix
		if s.doc.Nodes[id] == nil {
			return id
		}
	}
}

func (s *Stream) descendant(root, id string) bool {
	if root == id {
		return true
	}
	if node := s.doc.Nodes[root]; node != nil {
		for _, child := range node.Children {
			if s.descendant(child, id) {
				return true
			}
		}
	}
	return false
}

func (s *Stream) depth(id string) int {
	for _, node := range s.doc.Nodes {
		if slices.Contains(node.Children, id) {
			return 1 + s.depth(node.ID)
		}
	}
	return 1
}

// readProps keeps a block readable when a model writes text straight into props;
// only kinds that render text take a bare string.
func readProps(value any, kind string) (map[string]any, error) {
	switch props := value.(type) {
	case nil:
		return map[string]any{}, nil
	case string:
		if kind != "" && !slices.Contains(textKinds, kind) {
			return nil, fmt.Errorf("props")
		}
		out := map[string]any{"text": props}
		if data, _ := json.Marshal(out); len(data) > maxProps {
			return nil, fmt.Errorf("props")
		}
		return out, nil
	case map[string]any:
		if data, _ := json.Marshal(props); len(data) > maxProps {
			return nil, fmt.Errorf("props")
		}
		return props, nil
	}
	return nil, fmt.Errorf("props")
}

// String reads a string prop.
func String(n *Node, key, fallback string) string {
	if value, ok := n.Props[key].(string); ok {
		return value
	}
	return fallback
}

// Number reads a finite number prop.
func Number(n *Node, key string, fallback float64) float64 {
	if value, ok := n.Props[key].(float64); ok && !math.IsInf(value, 0) && !math.IsNaN(value) {
		return value
	}
	return fallback
}

// Option is one choice of a select.
type Option struct{ Label, Value string }

// Options reads a select's choices.
func Options(n *Node) []Option {
	list, _ := n.Props["options"].([]any)
	var out []Option
	for _, raw := range list[:min(len(list), 100)] {
		item, _ := raw.(map[string]any)
		label, hasLabel := item["label"].(string)
		value, hasValue := item["value"].(string)
		if hasLabel && hasValue {
			out = append(out, Option{label, value})
		}
	}
	return out
}

func isValue(v any) bool {
	switch value := v.(type) {
	case string, bool:
		return true
	case float64:
		return !math.IsInf(value, 0) && !math.IsNaN(value)
	}
	return false
}

// Defaults are the initial values of a document's controls.
func Defaults(doc *Document) map[string]any {
	out := map[string]any{}
	for _, node := range doc.Nodes {
		if !slices.Contains([]string{"input", "range", "select", "check", "tabs", "details"}, node.Kind) {
			continue
		}
		value := node.Props["default"]
		if !isValue(value) {
			switch node.Kind {
			case "check", "details":
				value = false
			case "range":
				value = Number(node, "min", 0)
			default:
				value = ""
			}
		}
		out[String(node, "bind", node.ID)] = value
	}
	return out
}

// Resolved are the values of inputs and calculated nodes. Forward references
// work; cycles and missing values stay unavailable, never zero.
func Resolved(doc *Document, edited map[string]any) map[string]any {
	values := Defaults(doc)
	for key, value := range edited {
		values[key] = value
	}
	visiting := map[string]bool{}
	var resolve func(string) any
	resolve = func(id string) any {
		if value, has := values[id]; has {
			return value
		}
		if visiting[id] || len(visiting) >= maxDepth {
			return nil
		}
		node := doc.Nodes[id]
		if node == nil || !slices.Contains([]string{"stat", "text", "heading"}, node.Kind) {
			return nil
		}
		visiting[id] = true
		expression := node.Props["value"]
		if expression == nil {
			expression = node.Props["text"]
		}
		result := evaluate(expression, values, 0, resolve)
		delete(visiting, id)
		values[id] = result
		return result
	}
	for id := range doc.Nodes {
		resolve(id)
	}
	return values
}

// Evaluate is whitelisted arithmetic, never code.
func Evaluate(expression any, values map[string]any) any {
	return evaluate(expression, values, 0, nil)
}

func evaluate(expression any, values map[string]any, depth int, resolve func(string) any) any {
	if isValue(expression) {
		return expression
	}
	record, isMap := expression.(map[string]any)
	if !isMap || depth > 16 {
		return nil
	}
	if ref, isRef := record["ref"].(string); isRef {
		if value, has := values[ref]; has {
			return value
		}
		if resolve != nil {
			return resolve(ref)
		}
		return nil
	}
	list, isList := record["args"].([]any)
	if !isList || len(list) > 32 {
		return nil
	}
	eval := func(x any) any { return evaluate(x, values, depth+1, resolve) }
	operator, _ := record["op"].(string)
	// Only the selected branch runs: an unused forward reference is harmless.
	if operator == "if" {
		if len(list) != 3 {
			return nil
		}
		condition := eval(list[0])
		if condition == nil {
			return nil
		}
		if truthy(condition) {
			return eval(list[1])
		}
		return eval(list[2])
	}
	args := make([]any, len(list))
	for i, item := range list {
		if args[i] = eval(item); args[i] == nil {
			return nil
		}
	}
	if operator == "concat" {
		var out strings.Builder
		for _, arg := range args {
			out.WriteString(Text(arg))
		}
		if len(utf16.Encode([]rune(out.String()))) > maxProps {
			return nil
		}
		return out.String()
	}
	if slices.Contains([]string{"eq", "lt", "gt", "sub", "div"}, operator) && len(args) != 2 {
		return nil
	}
	if operator == "eq" {
		return args[0] == args[1]
	}
	if len(args) == 0 {
		return nil
	}
	numbers := make([]float64, len(args))
	for i, arg := range args {
		value, ok := toNumber(arg)
		if !ok {
			return nil
		}
		numbers[i] = value
	}
	var result float64
	switch operator {
	case "add":
		for _, n := range numbers {
			result += n
		}
	case "mul":
		result = 1
		for _, n := range numbers {
			result *= n
		}
	case "sub":
		result = numbers[0] - numbers[1]
	case "div":
		result = numbers[0] / numbers[1]
	case "min":
		result = slices.Min(numbers)
	case "max":
		result = slices.Max(numbers)
	case "lt":
		return numbers[0] < numbers[1]
	case "gt":
		return numbers[0] > numbers[1]
	default:
		return nil
	}
	if math.IsInf(result, 0) || math.IsNaN(result) {
		return nil
	}
	return result
}

func truthy(v any) bool {
	switch value := v.(type) {
	case bool:
		return value
	case float64:
		return value != 0
	case string:
		return value != ""
	}
	return false
}

func toNumber(v any) (float64, bool) {
	switch value := v.(type) {
	case float64:
		return value, true
	case bool:
		if value {
			return 1, true
		}
		return 0, true
	case string:
		if strings.TrimSpace(value) == "" {
			return 0, false
		}
		n, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
		return n, err == nil && !math.IsInf(n, 0) && !math.IsNaN(n)
	}
	return 0, false
}

// Text is a value as a reader sees it.
func Text(v any) string {
	switch value := v.(type) {
	case float64:
		return strconv.FormatFloat(value, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(value)
	case string:
		return value
	}
	return ""
}

func nodeValue(n *Node, values map[string]any) any {
	expression := n.Props["value"]
	if expression == nil {
		expression = n.Props["text"]
	}
	if expression == nil {
		expression = ""
	}
	return Evaluate(expression, values)
}

// Label is a calculated node's shown text.
func Label(n *Node, values map[string]any) string {
	result := nodeValue(n, values)
	if result == nil {
		return "—"
	}
	return String(n, "prefix", "") + Text(result) + String(n, "suffix", "")
}

// CalculationUnavailable reports a node that cannot be shown yet: the rest of the
// answer stays usable.
func CalculationUnavailable(n *Node, values map[string]any) bool {
	if slices.Contains([]string{"stat", "text", "heading"}, n.Kind) {
		return nodeValue(n, values) == nil
	}
	list, isList := n.Props["data"].([]any)
	if n.Kind != "chart" || !isList {
		return false
	}
	for _, raw := range list {
		item, isMap := raw.(map[string]any)
		if !isMap {
			return true
		}
		if _, ok := toNumber(Evaluate(item["value"], values)); !ok {
			return true
		}
	}
	return false
}

// Action is what a set button or form changes; an invalid one changes nothing.
func Action(n *Node, values map[string]any) map[string]any {
	action, _ := n.Props["set"].(map[string]any)
	changes := map[string]any{}
	count := 0
	for key, expression := range action {
		if !idPattern.MatchString(key) || count >= 100 {
			continue
		}
		count++
		result := Evaluate(expression, values)
		if result == nil {
			return map[string]any{}
		}
		changes[key] = result
	}
	return changes
}

// SendText is the visible message an explicit click may submit. Generated content
// cannot select a recipient, tool or command.
func SendText(n *Node, values map[string]any) string {
	if n.Kind != "button" && n.Kind != "form" {
		return ""
	}
	text, _ := Evaluate(n.Props["send"], values).(string)
	text = strings.TrimSpace(text)
	if text == "" || len(utf16.Encode([]rune(text))) > 16_000 {
		return ""
	}
	return text
}

// Markdown routes media and code through the page's Markdown trust path; false for
// kinds with their own view.
func Markdown(n *Node) (string, bool) {
	text := String(n, "text", "")
	fenced := func(language string) string {
		longest := 2
		for _, run := range regexp.MustCompile("`+").FindAllString(text, -1) {
			longest = max(longest, len(run))
		}
		fence := strings.Repeat("`", longest+1)
		return fence + language + "\n" + text + "\n" + fence
	}
	switch n.Kind {
	case "markdown":
		return text, true
	case "code":
		return fenced(regexp.MustCompile(`[^\w+-]`).ReplaceAllString(String(n, "language", ""), "")), true
	case "diagram":
		return fenced("mermaid"), true
	case "math":
		return "$$\n" + text + "\n$$", true
	case "image", "video", "audio":
		src := strings.NewReplacer("\n", "", "\r", "", "<", "", ">", "").Replace(String(n, "src", ""))
		alt := strings.NewReplacer("[", "", "]", "", `\`, "", "\n", "", "\r", "").Replace(String(n, "alt", ""))
		if src == "" {
			return "", true
		}
		return "![" + alt + "](<" + src + ">)", true
	}
	return "", false
}

// Point is a value of a chart.
type Point struct {
	Label string
	Value float64
}

// Chart reads a chart's points.
func Chart(n *Node, values map[string]any) []Point {
	list, _ := n.Props["data"].([]any)
	var out []Point
	for _, raw := range list[:min(len(list), 100)] {
		item, isMap := raw.(map[string]any)
		if !isMap {
			continue
		}
		label, _ := item["label"].(string)
		if value, ok := toNumber(Evaluate(item["value"], values)); ok {
			out = append(out, Point{label, value})
		}
	}
	return out
}

// Range clamps a slider's value.
func Range(n *Node, current any) (lo, hi, step, value float64) {
	lo = Number(n, "min", 0)
	hi = max(lo+1, Number(n, "max", 100))
	step = max(math.SmallestNonzeroFloat64, Number(n, "step", 1))
	number, ok := toNumber(current)
	if !ok {
		number = lo
	}
	return lo, hi, step, min(hi, max(lo, number))
}

// LineTick and LinePoint are positions of a line chart.
type LineTick struct {
	Value, Y float64
	Label    string
}
type LinePoint struct {
	Point
	X, Y float64
}
type LineLabel struct {
	Label string
	X     float64
}

// LinePlot is richLinePlot: the same coordinates on every client.
type LinePlot struct {
	Width, Height, LabelY, Left, Right float64
	Ticks                              []LineTick
	Points                             []LinePoint
	Labels                             []LineLabel
	ShowValues                         bool
}

// PlotLine lays a line chart out in width.
func PlotLine(data []Point, width float64, options map[string]any) LinePlot {
	number := func(key string, fallback float64) float64 {
		if value, ok := options[key].(float64); ok && !math.IsInf(value, 0) && !math.IsNaN(value) {
			return value
		}
		return fallback
	}
	w, height := max(180, width), max(80, number("height", 220))
	top, bottom := max(0, number("top", 28)), height-max(0, number("bottom", 28))
	lo, hi := 0.0, 1.0
	for i, item := range data {
		if i == 0 {
			lo, hi = item.Value, item.Value
		}
		lo, hi = min(lo, item.Value), max(hi, item.Value)
	}
	if options["includeZero"] == true {
		lo, hi = min(0, lo), max(0, hi)
	}
	spread := hi - lo
	if spread == 0 {
		spread = math.Abs(hi) * 0.05
	}
	if spread == 0 {
		spread = 1
	}
	pad := spread * 0.1
	lo, hi = number("yMin", lo-pad), number("yMax", hi+pad)
	if hi <= lo {
		hi = lo + 1
	}
	scale := max(math.Abs(lo), math.Abs(hi), 1)
	span := hi/scale - lo/scale
	at := func(v float64) float64 { return bottom - (v/scale-lo/scale)/span*(bottom-top) }
	count := int(max(2, min(50, math.Round(number("ticks", 4)))))
	step := (hi - lo) / float64(count-1)
	decimals := int(max(0, min(12, number("decimals", math.Ceil(-math.Log10(step))+1))))
	plot := LinePlot{Width: w, Height: height, LabelY: height - 6, Right: max(0, number("right", 20))}
	longestTick := 0
	for i := range count {
		value := lo + step*float64(i)
		label := strconv.FormatFloat(value, 'f', decimals, 64)
		if strings.Contains(label, ".") {
			label = strings.TrimRight(strings.TrimRight(label, "0"), ".")
		}
		longestTick = max(longestTick, len(label))
		plot.Ticks = append(plot.Ticks, LineTick{value, at(value), label})
	}
	plot.Left = max(0, number("left", float64(longestTick*7+12)))
	space := (w - plot.Left - plot.Right) / max(1, float64(len(data)))
	longest, labelWidth := 1, 1
	for _, item := range data {
		longest = max(longest, len(strconv.FormatFloat(item.Value, 'f', -1, 64)))
		size := 0
		for _, r := range item.Label {
			if r > 255 {
				size += 12
			} else {
				size += 7
			}
		}
		labelWidth = max(labelWidth, size)
	}
	labelStep := int(max(1, math.Ceil(float64(labelWidth+12)/max(1, space))))
	for i, item := range data {
		x := plot.Left + (float64(i)+0.5)*space
		plot.Points = append(plot.Points, LinePoint{item, x, at(item.Value)})
		if options["showLabels"] != false && i%labelStep == 0 {
			plot.Labels = append(plot.Labels, LineLabel{item.Label, x})
		}
	}
	if show, ok := options["showValues"].(bool); ok {
		plot.ShowValues = show
	} else {
		plot.ShowValues = float64(longest*8+16) <= space
	}
	return plot
}

// Shape is a validated vector shape: only declarative values pass.
type Shape struct {
	Type  string
	Props map[string]any
	Text  string
}

var (
	shapeTypes   = []string{"path", "rect", "circle", "ellipse", "line", "polyline", "polygon", "text"}
	shapeNumbers = []string{"x", "y", "x1", "y1", "x2", "y2", "width", "height", "rx", "ry", "cx", "cy", "r", "strokeWidth", "opacity", "fillOpacity", "strokeOpacity", "fontSize", "letterSpacing"}
	pathData     = regexp.MustCompile(`^[MmZzLlHhVvCcSsQqTtAa\d.,+eE\s-]+$`)
	pointList    = regexp.MustCompile(`^[\d.,+eE\s-]+$`)
	transforms   = regexp.MustCompile(`^(?:(?:translate|scale|rotate|matrix|skewX|skewY)\([\d.,+eE\s-]+\)\s*)+$`)
)

// Vector reads an svg node: its view box and shapes.
func Vector(n *Node) (viewBox [4]float64, shapes []Shape) {
	viewBox = [4]float64{0, 0, 640, 360}
	var parts []float64
	switch value := n.Props["viewBox"].(type) {
	case []any:
		for _, item := range value {
			if number, ok := item.(float64); ok {
				parts = append(parts, number)
			}
		}
		if len(parts) != len(value) {
			parts = nil
		}
	case string:
		for _, field := range regexp.MustCompile(`[ ,]+`).Split(value, -1) {
			if number, err := strconv.ParseFloat(field, 64); err == nil {
				parts = append(parts, number)
			}
		}
	}
	if len(parts) == 4 && parts[2] > 0 && parts[3] > 0 {
		copy(viewBox[:], parts)
	}
	list, _ := n.Props["shapes"].([]any)
	for _, raw := range list[:min(len(list), 2048)] {
		item, isMap := raw.(map[string]any)
		kind, _ := item["type"].(string)
		if !isMap || !slices.Contains(shapeTypes, kind) {
			continue
		}
		shape := Shape{Type: kind, Props: map[string]any{}}
		if kind == "text" {
			shape.Text = Text(item["text"])
		}
		for key, value := range item {
			text, isText := value.(string)
			switch {
			case slices.Contains(shapeNumbers, key):
				if number, ok := value.(float64); ok {
					shape.Props[key] = number
				}
			case key == "fill" || key == "stroke":
				if isText {
					shape.Props[key] = text
				}
			case key == "d" && isText && pathData.MatchString(text),
				(key == "points" || key == "strokeDasharray") && isText && pointList.MatchString(text),
				key == "transform" && isText && transforms.MatchString(text),
				key == "textAnchor" && isText && slices.Contains([]string{"start", "middle", "end"}, text):
				shape.Props[key] = text
			}
		}
		shapes = append(shapes, shape)
	}
	return viewBox, shapes
}

// Style is the validated visual style of a node: colors are roles, never values.
type Style struct {
	Lengths map[string]float64
	Percent map[string]float64
	Text    map[string]string
	Color   string
	Opacity *float64
	Border  bool
}

var (
	styleLengths = []string{"width", "maxWidth", "minWidth", "height", "minHeight", "maxHeight", "padding", "paddingTop", "paddingRight", "paddingBottom", "paddingLeft", "marginTop", "marginBottom", "gap", "borderRadius", "fontSize", "lineHeight", "letterSpacing", "flexGrow", "flexShrink"}
	styleSizes   = []string{"width", "maxWidth", "minWidth", "height", "minHeight", "maxHeight"}
	styleEnums   = map[string][]string{
		"textAlign": {"left", "center", "right", "justify"}, "fontStyle": {"normal", "italic"},
		"alignItems": {"flex-start", "center", "flex-end", "stretch"}, "justifyContent": {"flex-start", "center", "flex-end", "space-between", "space-around", "space-evenly"},
		"flexDirection": {"row", "column", "row-reverse", "column-reverse"}, "flexWrap": {"wrap", "nowrap"},
		"textDecorationLine": {"none", "underline", "line-through"}, "borderStyle": {"solid", "dashed", "dotted"},
	}
	colorRoles  = []string{"primary", "secondary", "tertiary", "success", "warning", "danger"}
	percentSize = regexp.MustCompile(`^(\d+(?:\.\d+)?)%$`)
	fontWeight  = regexp.MustCompile(`^(?:[1-9]00|normal|bold)$`)
)

// ColorRole returns a named color role, or "".
func ColorRole(value any) string {
	role, _ := value.(string)
	if slices.Contains(colorRoles, role) {
		return role
	}
	return ""
}

// ReadStyle validates props.style.
func ReadStyle(n *Node) Style {
	out := Style{Lengths: map[string]float64{}, Percent: map[string]float64{}, Text: map[string]string{}}
	raw, _ := n.Props["style"].(map[string]any)
	for key, value := range raw {
		number, isNumber := value.(float64)
		text, isText := value.(string)
		switch {
		case key == "color":
			out.Color = ColorRole(value)
		case key == "borderWidth":
			out.Border = out.Border || isNumber && number > 0
		case slices.Contains(styleLengths, key) && isNumber:
			out.Lengths[key] = number
		case slices.Contains(styleSizes, key) && isText:
			if match := percentSize.FindStringSubmatch(text); match != nil {
				out.Percent[key], _ = strconv.ParseFloat(match[1], 64)
			}
		case key == "fontWeight" && fontWeight.MatchString(Text(value)):
			out.Text[key] = Text(value)
		case key == "opacity" && isNumber:
			clamped := max(0, min(1, number))
			out.Opacity = &clamped
		case slices.Contains(styleEnums[key], text):
			out.Text[key] = text
			out.Border = out.Border || key == "borderStyle"
		}
	}
	return out
}
