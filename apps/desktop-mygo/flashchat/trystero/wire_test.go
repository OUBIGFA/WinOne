package trystero

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"
)

// Vectors computed with @trystero-p2p/core 0.26.0 as the React frontend
// installs it (crypto.ts, action-wire.ts), for these inputs.
const (
	vectorApp      = "winone-interconnect-chitchatter"
	vectorRoom     = "3f1c2a9e-7b4d-4e7a-9c1f-2b8d6e5a4c3b"
	vectorPassword = "pässwörd-9"
)

func TestTopicsMatchTrystero(t *testing.T) {
	root := sha1Topic(topicPath(libName, vectorApp, vectorRoom))
	if root != "53i6o3d2h4s4fu4j643v2a3k1v503qn704044" {
		t.Errorf("root topic %s", root)
	}
	if got := infoHash(root); got != "655x4363z294d2k296nd" {
		t.Errorf("info hash %s", got)
	}
	if got := sha1Topic(topicPath(libName, vectorApp, vectorRoom, "AbCdEfGhIjKlMnOpQrSt")); got != "485z3n113zg1a265s3c6y1r2v173g5h2z6c31u" {
		t.Errorf("self topic %s", got)
	}
	if got := roomNamespace(vectorApp, vectorRoom); got != "7293b0caa48bcee8375d7a3d636ea498b58758a475e3f13adb58fc231ae350f8" {
		t.Errorf("namespace %s", got)
	}
	if got := handshakeHash("ChallengeChallengeChallengeChallenge", vectorPassword, vectorApp, vectorRoom); got != "3916fbd8435223a5a0116986e4d72fae8e4b3180dc5f99b05d2005405ca64f4a" {
		t.Errorf("handshake hash %s", got)
	}
}

func TestSignalEncryptionInteroperates(t *testing.T) {
	aead, err := signalCipher(vectorPassword, vectorApp, vectorRoom)
	if err != nil {
		t.Fatal(err)
	}
	const plain = "v=0\r\no=- 46117 2 IN IP4 127.0.0.1\r\ns=中文\r\n"
	// Encrypted by the browser's WebCrypto.
	browser := "79,108,196,211,154,20,199,149,74,163,181,71,66,250,251,147$xfAdc5VuJRs0VXfnF+GTBLw/QiRA/z/0mQmomCBwjcJssJV7YrRua4/9D9XEePeVW7GYdO020nN8/tmhig=="
	if got, err := decryptSignal(aead, browser); err != nil || got != plain {
		t.Fatalf("decrypt %q %v", got, err)
	}
	sealed, err := encryptSignal(aead, plain)
	if err != nil {
		t.Fatal(err)
	}
	iv, _, _ := strings.Cut(sealed, "$")
	if strings.Count(iv, ",") != 15 {
		t.Errorf("iv %q", iv)
	}
	if got, err := decryptSignal(aead, sealed); err != nil || got != plain {
		t.Fatalf("round trip %q %v", got, err)
	}
	// Another password cannot read it: the peers never connect.
	other, _ := signalCipher("other", vectorApp, vectorRoom)
	if _, err := decryptSignal(other, browser); err == nil {
		t.Error("decrypted with the wrong password")
	}
	for _, bad := range []string{"", "1,2$abc", browser[:20], strings.Replace(browser, "$", "", 1)} {
		if _, err := decryptSignal(aead, bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestGenIDUsesTrysteroAlphabet(t *testing.T) {
	id := genID(20)
	if len(id) != 20 || strings.Trim(id, charSet) != "" {
		t.Errorf("id %q", id)
	}
}

func hexBytes(t *testing.T, s string) []byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestInlineFramesMatchTrystero(t *testing.T) {
	type message struct {
		ID       string  `json:"id"`
		Text     string  `json:"text"`
		TimeSent float64 `json:"timeSent"`
		AuthorID string  `json:"authorId"`
	}
	payload, _ := marshalJSON(message{ID: "m1", Text: "你好 <b>", TimeSent: 1730000000000, AuthorID: "u1"})
	cases := []struct {
		format  byte
		payload []byte
		want    string
	}{
		{FormatJSON, payload, "0210672e30000000000000000000000000000000000000000000000000000000000000007b226964223a226d31222c2274657874223a22e4bda0e5a5bd203c623e222c2274696d6553656e74223a313733303030303030303030302c22617574686f724964223a227531227d"},
		{FormatString, nil, "0200672e3000000000000000000000000000000000000000000000000000000000000000"},
		{FormatJSON, []byte("null"), "0210672e30000000000000000000000000000000000000000000000000000000000000006e756c6c"},
		{FormatBinary, []byte{1, 2, 3}, "0220672e3000000000000000000000000000000000000000000000000000000000000000010203"},
	}
	for _, c := range cases {
		frame, inline, err := encodeStart("g.0", c.format, c.payload, nil, 0)
		if err != nil || !inline || hex.EncodeToString(frame) != c.want {
			t.Errorf("frame %x inline %v err %v\nwant %s", frame, inline, err, c.want)
		}
		f, err := decodeAction(hexBytes(t, c.want))
		if err != nil || f.kind != kindInline || f.typ != "g.0" || f.format != c.format || !bytes.Equal(f.payload, c.payload) {
			t.Errorf("decoded %+v %v", f, err)
		}
	}
}

func TestChunkedFramesMatchTrystero(t *testing.T) {
	big := bytes.Repeat([]byte("x"), 40000)
	frame, inline, err := encodeStart("g.0", FormatString, big, []byte(`{"r":"abc"}`), 0)
	want := "0201672e300000000000000000000000000000000000000000000000000000000000000b0000000040e38800000000007b2272223a22616263227d"
	if err != nil || inline || hex.EncodeToString(frame) != want {
		t.Fatalf("offer %x %v %v", frame, inline, err)
	}
	f, err := decodeAction(frame)
	if err != nil || f.kind != kindOffer || f.size != 40000 || string(f.meta) != `{"r":"abc"}` {
		t.Fatalf("decoded offer %+v %v", f, err)
	}
	// Chunks start with version, kind, id and the offset as a float64.
	starts := []string{"0202000000000000000000000000787878787878", "02020000000040cff90000000000787878787878", "02020000000040dff90000000000787878787878"}
	sizes := []int{16384, 16384, 7274}
	for i, offset := 0, 0; offset < len(big); i, offset = i+1, offset+chunkSize {
		chunk := chunkFrame(0, offset, big[offset:min(offset+chunkSize, len(big))])
		if hex.EncodeToString(chunk[:20]) != starts[i] || len(chunk) != sizes[i] {
			t.Errorf("chunk %d %x… %d", i, chunk[:20], len(chunk))
		}
		d, err := decodeAction(chunk)
		if err != nil || d.kind != kindChunk || d.offset != float64(offset) {
			t.Errorf("decoded chunk %d %v", i, err)
		}
	}
	accepted := controlFrame(kindAccepted, 7, "")
	if d, err := decodeAction(accepted); err != nil || d.kind != kindAccepted || d.id != 7 || len(accepted) != controlHeaderSize {
		t.Errorf("accepted %x %v", accepted, err)
	}
	rejected := controlFrame(kindRejected, 7, strings.Repeat("r", 300))
	if d, err := decodeAction(rejected); err != nil || len(d.reason) != 200 {
		t.Errorf("rejected %v", err)
	}
}

func TestInvalidFramesAreRefused(t *testing.T) {
	for _, bad := range [][]byte{
		nil, {2}, {1, 0}, // too short, wrong version
		append([]byte{2, 0x30}, make([]byte, 34)...),                 // format 3
		append([]byte{2, 0x13}, make([]byte, 4)...),                  // control with a format
		append(append([]byte{2, 0}, make([]byte, 32)...), 0, 0),      // no type
		append(append([]byte{2, 0, 'g'}, make([]byte, 31)...), 0, 9), // metadata past the end
		bytes.Repeat([]byte{2}, maxActionFrameBytes+1),
	} {
		if _, err := decodeAction(bad); err == nil {
			t.Errorf("accepted %x", bad)
		}
	}
	if _, _, err := encodeStart(strings.Repeat("t", 33), 0, nil, nil, 0); err == nil {
		t.Error("type of 33 bytes accepted")
	}
}

func TestRoomFramesCarryTheToken(t *testing.T) {
	token := roomNamespace(vectorApp, vectorRoom)
	frame := wrapRoomFrame(token, []byte{2, 0})
	if frame[0] != 1 || frame[1] != 0 || frame[2] != 64 || string(frame[3:67]) != token {
		t.Fatalf("frame %x", frame[:8])
	}
	f, ok := unwrapRoomFrame(frame)
	if !ok || f.presence || f.token != token || !bytes.Equal(f.payload, []byte{2, 0}) {
		t.Fatalf("unwrapped %+v %v", f, ok)
	}
	p := presenceFrame(token, true)
	if p[0] != 2 || p[1] != 1 || p[3] != 64 {
		t.Fatalf("presence %x", p[:4])
	}
	if f, ok := unwrapRoomFrame(p); !ok || !f.presence || !f.present || f.token != token {
		t.Fatalf("presence %+v", f)
	}
	if f, ok := unwrapRoomFrame(presenceFrame(token, false)); !ok || f.present {
		t.Fatal("absence read as presence")
	}
	for _, bad := range [][]byte{{1, 0}, {3, 0, 1, 'x'}, {1, 0, 0}, {1, 0, 200, 'x'}} {
		if _, ok := unwrapRoomFrame(bad); ok {
			t.Errorf("accepted %x", bad)
		}
	}
}

func TestRTCConfigTakesBrowserShapes(t *testing.T) {
	c, err := ParseRTCConfig(`{"iceServers":[{"urls":"turn:example.com:3478","username":"u","credential":"p"},{"urls":["stun:a","stun:b"]}],"iceTransportPolicy":"relay"}`)
	if err != nil || len(c.ICEServers) != 2 || c.ICEServers[0].URLs[0] != "turn:example.com:3478" || c.ICEServers[1].URLs[1] != "stun:b" || c.ICETransportPolicy != "relay" {
		t.Fatalf("%+v %v", c, err)
	}
	if patternFor(c.iceServers()) != relayCandidate {
		t.Error("TURN does not wait for relay candidates")
	}
	empty, _ := ParseRTCConfig(`{"iceServers":[]}`)
	if len(empty.iceServers()) != 0 || patternFor(empty.iceServers()) != anyCandidate {
		t.Error("an empty list must replace the defaults")
	}
	var none *RTCConfig
	if len(none.iceServers()) != 4 || patternFor(none.iceServers()) != reflexiveCandidate {
		t.Error("defaults")
	}
	for _, bad := range []string{`[]`, `1`, `{"iceServers":{}}`, `{"iceServers":[{"urls":3}]}`, `{"iceTransportPolicy":"none"}`, `nope`} {
		if _, err := ParseRTCConfig(bad); err == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestRelaysFollowTrysteroDefaults(t *testing.T) {
	got := Config{Redundancy: 4}.relays()
	want := []string{"wss://open.ftorrent.com", "wss://tracker.webtorrent.dev", "wss://tracker.openwebtorrent.com", "wss://tracker.btorrent.xyz"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("relays %v", got)
	}
	if got := (Config{RelayURLs: []string{"ws://127.0.0.1:1"}, Redundancy: 4}).relays(); len(got) != 1 {
		t.Errorf("custom relays %v", got)
	}
	for text, ok := range map[string]bool{"wss://t.example": true, "ws://127.0.0.1:8000": true, "ws://localhost:8000": false, "http://x": false, "wss://": false, "": false} {
		if ValidRelayURL(text) != ok {
			t.Errorf("%q valid %v", text, !ok)
		}
	}
}
