package learning

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Online struct{ VoiceURL, DictionaryURL, CacheDirectory string }

func fetch(ctx context.Context, address string, limit int64) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return nil, "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("在线服务返回 HTTP %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > limit {
		return nil, "", errors.New("在线服务返回内容过大")
	}
	return data, response.Header.Get("Content-Type"), nil
}
func validAudio(data []byte) bool {
	return len(data) >= 3 && (string(data[:3]) == "ID3" || data[0] == 0xff && data[1]&0xe0 == 0xe0)
}
func (o Online) Pronunciation(ctx context.Context, text, accent string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if accent != "uk" && accent != "us" {
		return nil, errors.New("未知发音类型")
	}
	if kind := Classify(text); kind != Word && kind != Phrase {
		return nil, errors.New("真人发音仅用于单词或短语")
	}
	sum := sha256.Sum256([]byte(accent + "\x00" + strings.ToLower(strings.TrimSpace(text))))
	path := ""
	if o.CacheDirectory != "" {
		path = filepath.Join(o.CacheDirectory, fmt.Sprintf("%x.mp3", sum))
		if data, err := os.ReadFile(path); err == nil && validAudio(data) {
			return data, nil
		}
	}
	endpoint := o.VoiceURL
	if endpoint == "" {
		endpoint = "https://dict.youdao.com/dictvoice"
	}
	query := url.Values{"audio": {text}, "type": {"2"}}
	if accent == "uk" {
		query.Set("type", "1")
	}
	data, content, err := fetch(ctx, endpoint+"?"+query.Encode(), 2<<20)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(content, "audio/") || !validAudio(data) {
		return nil, errors.New("真人发音未返回有效 MP3")
	}
	if path != "" {
		if err = os.MkdirAll(o.CacheDirectory, 0700); err != nil {
			return nil, fmt.Errorf("无法创建发音缓存: %w", err)
		}
		files, err := os.ReadDir(o.CacheDirectory)
		if err != nil {
			return nil, err
		}
		// A bounded cache avoids accumulating a user's reading history forever.
		// At capacity playback still works, without removing existing user files.
		if len(files) < 256 {
			if err = os.WriteFile(path, data, 0600); err != nil {
				return nil, fmt.Errorf("无法保存发音缓存: %w", err)
			}
		}
	}
	return data, nil
}

type OnlineDefinition struct {
	Definition string `json:"definition"`
	Example    string `json:"example"`
}
type OnlineMeaning struct {
	PartOfSpeech string             `json:"partOfSpeech"`
	Definitions  []OnlineDefinition `json:"definitions"`
}
type OnlineEntry struct {
	Word       string          `json:"word"`
	Phonetic   string          `json:"phonetic"`
	Meanings   []OnlineMeaning `json:"meanings"`
	SourceURLs []string        `json:"sourceUrls"`
	License    struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"license"`
}

func (o Online) Lookup(ctx context.Context, word string) ([]OnlineEntry, error) {
	endpoint := o.DictionaryURL
	if endpoint == "" {
		endpoint = "https://api.dictionaryapi.dev/api/v2/entries/en"
	}
	data, _, err := fetch(ctx, strings.TrimRight(endpoint, "/")+"/"+url.PathEscape(word), 1<<20)
	if err != nil {
		return nil, err
	}
	var entries []OnlineEntry
	if err = json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("英英词典返回格式无效: %w", err)
	}
	if len(entries) == 0 {
		return nil, errors.New("英英词典未收录此词")
	}
	return entries, nil
}
