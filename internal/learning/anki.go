package learning

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const AnkiURL = "http://127.0.0.1:8765"
const AnkiInstallURL = "https://ankiweb.net/shared/info/2055492159"
const AnkiDownloadURL = "https://apps.ankiweb.net/"
const FSRSURL = "https://docs.ankiweb.net/deck-options.html#fsrs"

// Endpoint exists for isolated profiles and contract tests. Only literal local
// addresses are accepted, and the transport neither uses proxies nor redirects.
type Anki struct {
	Endpoint string
	// Call routes bundled Anki through the verified WinOne Host. Endpoint is
	// retained for explicitly selected external compatibility and contract tests.
	Call func(context.Context, string, any, any) error
}

func (a Anki) invoke(ctx context.Context, action string, params any, out any) error {
	if a.Call != nil {
		return a.Call(ctx, action, params, out)
	}
	endpoint := a.Endpoint
	if endpoint == "" {
		endpoint = AnkiURL
	}
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.Scheme != "http" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("AnkiConnect 地址必须是本机 HTTP 地址")
	}
	ip := net.ParseIP(parsed.Hostname())
	if ip == nil || !ip.IsLoopback() {
		return errors.New("AnkiConnect 仅允许本机连接")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	// API 6 validates params as an object, including actions with no arguments.
	if params == nil {
		params = map[string]any{}
	}
	body, err := json.Marshal(map[string]any{"action": action, "version": 6, "params": params})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	transport := &http.Transport{Proxy: nil}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("AnkiConnect 不允许重定向") }}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("无法连接 Anki，请打开 Anki 并启用 AnkiConnect: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("AnkiConnect 返回 HTTP %d", response.StatusCode)
	}
	var envelope map[string]json.RawMessage
	if err = json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&envelope); err != nil {
		return fmt.Errorf("AnkiConnect 返回无效内容: %w", err)
	}
	failure, hasError := envelope["error"]
	result, hasResult := envelope["result"]
	if !hasError || !hasResult {
		return errors.New("AnkiConnect 返回格式不完整")
	}
	if string(failure) != "null" {
		var detail string
		if json.Unmarshal(failure, &detail) != nil {
			detail = "未知错误"
		}
		return fmt.Errorf("AnkiConnect: %s", detail)
	}
	if out != nil {
		if err = json.Unmarshal(result, out); err != nil {
			return fmt.Errorf("AnkiConnect 返回值无效: %w", err)
		}
	}
	return nil
}

type AnkiStatus struct {
	Version int
	Decks   []string
}

func (a Anki) Status(ctx context.Context) (AnkiStatus, error) {
	var s AnkiStatus
	if err := a.invoke(ctx, "version", nil, &s.Version); err != nil {
		return s, err
	}
	if s.Version < 6 {
		return s, errors.New("请更新 AnkiConnect 至支持 API 6 的版本")
	}
	err := a.invoke(ctx, "deckNames", nil, &s.Decks)
	return s, err
}
func searchTerm(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `*`, `\*`, `_`, `\_`).Replace(value) + `"`
}
func (a Anki) Browse(ctx context.Context, deck, word string) error {
	query := searchTerm("deck:" + deck)
	if word != "" {
		query += " " + searchTerm("Word:"+word)
	}
	return a.invoke(ctx, "guiBrowse", map[string]any{"query": query}, nil)
}
func (a Anki) Review(ctx context.Context, deck string) error {
	var opened bool
	if err := a.invoke(ctx, "guiDeckReview", map[string]any{"name": deck}, &opened); err != nil {
		return err
	}
	if !opened {
		return errors.New("Anki 未打开复习，请检查目标牌组是否存在")
	}
	return nil
}

type Note struct {
	Word      string `json:"word"`
	Phonetic  string `json:"phonetic"`
	Meaning   string `json:"meaning"`
	Example   string `json:"example"`
	Etymology string `json:"etymology"`
}
type AddedNote struct {
	ID       int64
	Existing bool
}

var stripTags = regexp.MustCompile(`<[^>]*>`)

func escapedField(value string) string {
	return strings.ReplaceAll(html.EscapeString(value), "\n", "<br>")
}

func (a Anki) Add(ctx context.Context, o Options, n Note, audio []byte) (AddedNote, error) {
	var result AddedNote
	if err := o.Validate(); err != nil {
		return result, err
	}
	n.Word = strings.TrimSpace(n.Word)
	if n.Word == "" || len(n.Word) > 4096 || strings.TrimSpace(n.Meaning) == "" {
		return result, errors.New("建卡前请填写词条和释义")
	}
	// Anki field searches are not exact. Verify the returned field before
	// suppressing a new card, preserving punctuation and HTML entities.
	var ids []int64
	if err := a.invoke(ctx, "findNotes", map[string]any{"query": searchTerm("deck:"+o.Deck) + " " + searchTerm("Word:"+n.Word)}, &ids); err != nil {
		return result, err
	}
	if len(ids) > 0 {
		var notes []struct {
			ID     int64 `json:"noteId"`
			Fields map[string]struct {
				Value string `json:"value"`
			} `json:"fields"`
		}
		if err := a.invoke(ctx, "notesInfo", map[string]any{"notes": ids}, &notes); err != nil {
			return result, err
		}
		for _, note := range notes {
			if strings.EqualFold(strings.TrimSpace(html.UnescapeString(stripTags.ReplaceAllString(note.Fields["Word"].Value, ""))), n.Word) {
				return AddedNote{ID: note.ID, Existing: true}, nil
			}
		}
	}
	model := "WinOne Word"
	if o.CardMode == "typing" {
		model += " Typing"
	}
	var models []string
	if err := a.invoke(ctx, "modelNames", nil, &models); err != nil {
		return result, err
	}
	fields := []string{"Word", "Phonetic", "Meaning", "Example", "Etymology", "Audio"}
	exists := false
	for _, name := range models {
		if name == model {
			exists = true
		}
	}
	if !exists {
		front := "{{Word}}"
		back := "{{FrontSide}}<hr id=answer>{{Phonetic}}<br>{{Meaning}}<br>{{Example}}<br>{{Etymology}}<br>{{Audio}}"
		if o.CardMode == "typing" {
			front = "{{Meaning}}<br>{{type:Word}}"
			back = "{{FrontSide}}<hr id=answer>{{type:Word}}<br>{{Phonetic}}<br>{{Example}}<br>{{Etymology}}<br>{{Audio}}"
		}
		if err := a.invoke(ctx, "createModel", map[string]any{"modelName": model, "inOrderFields": fields, "css": ".card { font-family: Arial, sans-serif; font-size: 22px; text-align: left; line-height: 1.6; }", "cardTemplates": []map[string]string{{"Name": "WinOne", "Front": front, "Back": back}}}, nil); err != nil {
			return result, err
		}
	} else {
		var existing []string
		if err := a.invoke(ctx, "modelFieldNames", map[string]any{"modelName": model}, &existing); err != nil {
			return result, err
		}
		for _, field := range fields {
			found := false
			for _, value := range existing {
				if value == field {
					found = true
				}
			}
			if !found {
				return result, fmt.Errorf("Anki 笔记类型 %s 缺少字段 %s；请在 Anki 中修复，WinOne 不会覆盖已有模板", model, field)
			}
		}
	}
	if err := a.invoke(ctx, "createDeck", map[string]any{"deck": o.Deck}, nil); err != nil {
		return result, err
	}
	values := map[string]string{"Word": escapedField(n.Word), "Phonetic": escapedField(n.Phonetic), "Meaning": escapedField(n.Meaning), "Example": escapedField(n.Example), "Etymology": escapedField(n.Etymology), "Audio": ""}
	if len(audio) > 0 {
		sum := sha256.Sum256(audio)
		filename := fmt.Sprintf("winone-%x.mp3", sum[:12])
		var stored string
		if err := a.invoke(ctx, "storeMediaFile", map[string]any{"filename": filename, "data": base64.StdEncoding.EncodeToString(audio)}, &stored); err != nil {
			return result, err
		}
		if stored != filename {
			return result, errors.New("Anki 返回了不一致的音频文件名")
		}
		values["Audio"] = "[sound:" + stored + "]"
	}
	err := a.invoke(ctx, "addNote", map[string]any{"note": map[string]any{"deckName": o.Deck, "modelName": model, "fields": values, "tags": []string{"WinOne"}, "options": map[string]any{"allowDuplicate": false, "duplicateScope": "deck", "duplicateScopeOptions": map[string]any{"deckName": o.Deck, "checkChildren": false}}}}, &result.ID)
	if err == nil && result.ID <= 0 {
		err = errors.New("Anki 未返回已创建的笔记编号")
	}
	return result, err
}
