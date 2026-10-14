package resultui

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/gorilla/websocket"
)

// Edge Read Aloud protocol, also used by thukiwin and edge-tts. This public
// browser token is not an account credential. Text is sent only after a click.
const edgeSpeechEndpoint = "wss://speech.platform.bing.com/consumer/speech/synthesize/readaloud/edge/v1"
const edgeSpeechToken = "6A5AA1D4EAFF4E9FB37E23D68491D6F4"
const edgeSpeechVersion = "143.0.3650.75"
const edgeSpeechVoice = "en-US-AvaMultilingualNeural"
const speechChunkBytes = 3000

type edgeSynthesizer struct {
	endpoint string
	skew     atomic.Int64
}

var edgeSpeech = &edgeSynthesizer{endpoint: edgeSpeechEndpoint}

func readSpeech(ctx context.Context, text string, loading func(bool)) error {
	select {
	case speechLane <- struct{}{}:
		defer func() { <-speechLane }()
	case <-ctx.Done():
		return ctx.Err()
	}
	for _, chunk := range speechChunks(text, speechChunkBytes) {
		if err := ctx.Err(); err != nil {
			return err
		}
		loading(true)
		audio, err := edgeSpeech.synthesize(ctx, chunk)
		if err != nil {
			return err
		}
		loading(false)
		if err = playSpeech(ctx, audio); err != nil {
			return err
		}
	}
	return nil
}

// Preserve every UTF-8 byte, preferring sentence/word boundaries. The small
// byte budget remains below Edge's SSML limit even after XML escaping.
func speechChunks(text string, budget int) []string {
	budget = max(budget, utf8.UTFMax)
	var chunks []string
	for len(text) > budget {
		end, boundary := 0, 0
		for i, r := range text {
			next := i + utf8.RuneLen(r)
			if next > budget {
				break
			}
			end = next
			if unicode.IsSpace(r) || strings.ContainsRune("。！？.!?;；", r) {
				boundary = next
			}
		}
		if boundary >= budget/2 {
			end = boundary
		}
		chunks = append(chunks, text[:end])
		text = text[end:]
	}
	if text != "" {
		chunks = append(chunks, text)
	}
	return chunks
}

func speechGEC(now time.Time) string {
	seconds := now.Unix() + 11644473600 // Windows epoch (1601), in seconds.
	ticks := (seconds - seconds%300) * 10000000
	hash := sha256.Sum256([]byte(fmt.Sprintf("%d%s", ticks, edgeSpeechToken)))
	return strings.ToUpper(hex.EncodeToString(hash[:]))
}

func speechRequestID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(id[:]), nil
}

func (s *edgeSynthesizer) connect(ctx context.Context) (*websocket.Conn, error) {
	dialer := websocket.Dialer{Proxy: http.ProxyFromEnvironment, HandshakeTimeout: 15 * time.Second}
	// A server Date on a rejected handshake corrects local clock skew once;
	// connection/certificate failures are never hidden by another service.
	for attempt := 0; attempt < 2; attempt++ {
		id, err := speechRequestID()
		if err != nil {
			return nil, err
		}
		u, err := url.Parse(s.endpoint)
		if err != nil {
			return nil, err
		}
		query := u.Query()
		query.Set("TrustedClientToken", edgeSpeechToken)
		query.Set("ConnectionId", id)
		query.Set("Sec-MS-GEC", speechGEC(time.Now().Add(time.Duration(s.skew.Load())*time.Second)))
		query.Set("Sec-MS-GEC-Version", "1-"+edgeSpeechVersion)
		u.RawQuery = query.Encode()
		headers := http.Header{
			"Origin":     {"chrome-extension://jdiccldimpdaibmpdkjnbmckianbfold"},
			"User-Agent": {"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36 Edg/143.0.0.0"},
			"Pragma":     {"no-cache"}, "Cache-Control": {"no-cache"},
			"Accept-Language": {"en-US,en;q=0.9"},
			"Cookie":          {"muid=" + strings.ToUpper(id) + ";"},
		}
		conn, response, err := dialer.DialContext(ctx, u.String(), headers)
		if err == nil {
			return conn, nil
		}
		if response != nil {
			response.Body.Close()
			if response.StatusCode == http.StatusForbidden && attempt == 0 {
				if serverTime, dateErr := http.ParseTime(response.Header.Get("Date")); dateErr == nil {
					s.skew.Store(serverTime.Unix() - time.Now().Unix())
					continue
				}
			}
			return nil, fmt.Errorf("speech service HTTP %d", response.StatusCode)
		}
		return nil, fmt.Errorf("speech service connection: %w", err)
	}
	return nil, errors.New("speech service rejected connection")
}

func speechSSML(text string) string {
	var escaped bytes.Buffer
	_ = xml.EscapeText(&escaped, []byte(text))
	// The multilingual voice detects the text language, including mixed text.
	return "<speak version='1.0' xmlns='http://www.w3.org/2001/10/synthesis' xml:lang='en-US'><voice name='" + edgeSpeechVoice + "'><prosody pitch='+0Hz' rate='+0%' volume='+0%'>" + escaped.String() + "</prosody></voice></speak>"
}

func speechHeader(header, name string) string {
	header, _, _ = strings.Cut(header, "\r\n\r\n")
	for _, line := range strings.Split(header, "\r\n") {
		key, value, ok := strings.Cut(line, ":")
		if ok && strings.EqualFold(key, name) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func speechAudioFrame(data []byte) ([]byte, error) {
	if len(data) < 2 {
		return nil, errors.New("truncated speech frame")
	}
	end := int(binary.BigEndian.Uint16(data[:2])) + 2
	if end > len(data) {
		return nil, errors.New("truncated speech header")
	}
	if speechHeader(string(data[2:end]), "Path") != "audio" {
		return nil, errors.New("unexpected speech frame")
	}
	return data[end:], nil
}

func (s *edgeSynthesizer) synthesize(parent context.Context, text string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()
	conn, err := s.connect(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	stopClose := context.AfterFunc(ctx, func() { conn.Close() })
	defer stopClose()
	conn.SetReadLimit(8 << 20)
	deadline, _ := ctx.Deadline()
	if err = conn.SetReadDeadline(deadline); err != nil {
		return nil, err
	}
	if err = conn.SetWriteDeadline(deadline); err != nil {
		return nil, err
	}
	id, err := speechRequestID()
	if err != nil {
		return nil, err
	}
	timestamp := time.Now().UTC().Format("Mon Jan 02 2006 15:04:05 GMT+0000 (Coordinated Universal Time)")
	config := "X-Timestamp:" + timestamp + "\r\nContent-Type:application/json; charset=utf-8\r\nPath:speech.config\r\n\r\n" +
		`{"context":{"synthesis":{"audio":{"metadataoptions":{"sentenceBoundaryEnabled":"false","wordBoundaryEnabled":"false"},"outputFormat":"audio-24khz-48kbitrate-mono-mp3"}}}}`
	ssml := "X-RequestId:" + id + "\r\nContent-Type:application/ssml+xml\r\nX-Timestamp:" + timestamp + "Z\r\nPath:ssml\r\n\r\n" + speechSSML(text)
	for _, message := range []string{config, ssml} {
		if err = conn.WriteMessage(websocket.TextMessage, []byte(message)); err != nil {
			return nil, err
		}
	}
	var audio []byte
	for {
		kind, data, err := conn.ReadMessage()
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, fmt.Errorf("incomplete speech response: %w", err)
		}
		if kind == websocket.TextMessage && speechHeader(string(data), "Path") == "turn.end" {
			if len(audio) == 0 {
				return nil, errors.New("speech service returned no audio")
			}
			return audio, nil
		}
		if kind == websocket.BinaryMessage {
			chunk, err := speechAudioFrame(data)
			if err != nil {
				return nil, err
			}
			if len(audio)+len(chunk) > 8<<20 {
				return nil, errors.New("speech response exceeds audio limit")
			}
			audio = append(audio, chunk...)
		}
	}
}
