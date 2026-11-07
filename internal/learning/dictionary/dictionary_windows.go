// Package dictionary reads the bundled ECDICT SQLite database through the
// Windows system SQLite library. Handles are per learning operation and closed
// immediately afterward; no background database service or driver is loaded.
package dictionary

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"winone/internal/learning"
)

var sqlite = windows.NewLazySystemDLL("winsqlite3.dll")
var openDB = sqlite.NewProc("sqlite3_open_v2")
var closeDB = sqlite.NewProc("sqlite3_close")
var prepare = sqlite.NewProc("sqlite3_prepare_v2")
var bindText = sqlite.NewProc("sqlite3_bind_text16")
var step = sqlite.NewProc("sqlite3_step")
var finalize = sqlite.NewProc("sqlite3_finalize")
var columnText = sqlite.NewProc("sqlite3_column_text16")
var columnBytes = sqlite.NewProc("sqlite3_column_bytes16")
var columnInt = sqlite.NewProc("sqlite3_column_int")
var copyMemory = windows.NewLazySystemDLL("kernel32.dll").NewProc("RtlMoveMemory")

type Dictionary struct {
	mu     sync.Mutex
	handle uintptr
}

func Open(path string) (*Dictionary, error) {
	if err := sqlite.Load(); err != nil {
		return nil, fmt.Errorf("系统 SQLite 不可用: %w", err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	name, err := windows.BytePtrFromString(absolute)
	if err != nil {
		return nil, err
	}
	d := &Dictionary{}
	// READONLY | FULLMUTEX; an absent file is never created.
	code, _, _ := openDB.Call(uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(&d.handle)), 1|0x10000, 0)
	runtime.KeepAlive(name)
	if code != 0 {
		if d.handle != 0 {
			closeDB.Call(d.handle)
		}
		return nil, fmt.Errorf("无法打开离线词典（SQLite %d），请修复 WinOne 资源", code)
	}
	return d, nil
}
func (d *Dictionary) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle == 0 {
		return nil
	}
	code, _, _ := closeDB.Call(d.handle)
	if code != 0 {
		return fmt.Errorf("关闭词典失败（SQLite %d）", code)
	}
	d.handle = 0
	return nil
}
func (d *Dictionary) query(ctx context.Context, sql, word string, read func(uintptr)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.handle == 0 {
		return errors.New("词典已关闭")
	}
	text, _ := windows.BytePtrFromString(sql)
	var statement uintptr
	code, _, _ := prepare.Call(d.handle, uintptr(unsafe.Pointer(text)), ^uintptr(0), uintptr(unsafe.Pointer(&statement)), 0)
	runtime.KeepAlive(text)
	if code != 0 {
		return fmt.Errorf("离线词典格式无效（SQLite %d）", code)
	}
	defer finalize.Call(statement)
	value, err := windows.UTF16PtrFromString(word)
	if err != nil {
		return err
	}
	code, _, _ = bindText.Call(statement, 1, uintptr(unsafe.Pointer(value)), ^uintptr(0), ^uintptr(0))
	runtime.KeepAlive(value)
	if code != 0 {
		return fmt.Errorf("查询词典失败（SQLite %d）", code)
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		code, _, _ = step.Call(statement)
		if code == 101 {
			return nil
		}
		if code != 100 {
			return fmt.Errorf("读取词典失败（SQLite %d）", code)
		}
		read(statement)
	}
}
func stringColumn(statement uintptr, index int) string {
	value, _, _ := columnText.Call(statement, uintptr(index))
	if value == 0 {
		return ""
	}
	length, _, _ := columnBytes.Call(statement, uintptr(index))
	if length == 0 {
		return ""
	}
	// The SQLite-owned address is not a Go pointer. Copy it through the native
	// ABI while the statement is live instead of converting a saved uintptr.
	text := make([]uint16, length/2+1)
	copyMemory.Call(uintptr(unsafe.Pointer(&text[0])), value, length)
	runtime.KeepAlive(text)
	return windows.UTF16ToString(text)
}
func intColumn(statement uintptr, index int) int {
	value, _, _ := columnInt.Call(statement, uintptr(index))
	return int(int32(value))
}

var tags = map[string]string{"zk": "中考", "gk": "高考", "cet4": "四级", "cet6": "六级", "ky": "考研", "toefl": "托福", "ielts": "雅思", "gre": "GRE"}

func (d *Dictionary) Lookup(ctx context.Context, raw string) ([]learning.Entry, error) {
	word := strings.ToLower(strings.TrimSpace(strings.ReplaceAll(raw, "’", "'")))
	var entries []learning.Entry
	err := d.query(ctx, `SELECT word,phonetic,definition,translation,tag,bnc,coca,collins,oxford,exchange FROM entries WHERE word=?1 OR word IN (SELECT lemma FROM forms WHERE form=?1) ORDER BY CASE WHEN word=?1 THEN 0 ELSE 1 END,word`, word, func(s uintptr) {
		e := learning.Entry{Word: stringColumn(s, 0), Phonetic: stringColumn(s, 1), Definition: stringColumn(s, 2), Translation: stringColumn(s, 3), BNC: intColumn(s, 5), COCA: intColumn(s, 6), Collins: intColumn(s, 7), Oxford: intColumn(s, 8) != 0, Forms: map[string]string{}}
		for _, tag := range strings.Fields(stringColumn(s, 4)) {
			if label, ok := tags[tag]; ok {
				e.Tags = append(e.Tags, label)
			}
		}
		for _, part := range strings.Split(stringColumn(s, 9), "/") {
			key, value, ok := strings.Cut(part, ":")
			if !ok {
				continue
			}
			if key == "0" {
				e.Lemmas = strings.Split(value, ",")
			} else if strings.Contains("pdi3rts", key) && len(key) == 1 {
				e.Forms[key] = value
			}
		}
		entries = append(entries, e)
	})
	return entries, err
}

var tokens = regexp.MustCompile(`[A-Za-z]+(?:[-'’][A-Za-z]+)*`)

// Extract returns all distinct eligible words, ranked by study relevance. The
// AI also receives the full passage to identify multiword phrases in context.
func (d *Dictionary) Extract(ctx context.Context, text, level string) ([]learning.Entry, error) {
	threshold := 4000
	if level == "beginner" {
		threshold = 1000
	}
	if level == "advanced" {
		threshold = 10000
	}
	seen := map[string]bool{}
	selected := map[string]learning.Entry{}
	for _, token := range tokens.FindAllString(strings.ToLower(text), -1) {
		if seen[token] || len(token) < 2 {
			continue
		}
		seen[token] = true
		entries, err := d.Lookup(ctx, token)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			rank := e.COCA
			if rank == 0 {
				rank = e.BNC
			}
			if rank > 0 && rank <= threshold {
				continue
			}
			if len(e.Tags) == 0 && !e.Oxford && e.Collins == 0 && rank == 0 {
				continue
			}
			selected[e.Word] = e
		}
	}
	result := make([]learning.Entry, 0, len(selected))
	for _, entry := range selected {
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool {
		a, b := result[i], result[j]
		if len(a.Tags) != len(b.Tags) {
			return len(a.Tags) > len(b.Tags)
		}
		ar, br := a.COCA, b.COCA
		if ar == 0 {
			ar = 1 << 30
		}
		if br == 0 {
			br = 1 << 30
		}
		if ar != br {
			return ar < br
		}
		return a.Word < b.Word
	})
	return result, nil
}

// RankLabel keeps unknown ranks distinct from the most frequent words.
func RankLabel(rank int) string {
	if rank <= 0 {
		return "—"
	}
	return strconv.Itoa(rank)
}
