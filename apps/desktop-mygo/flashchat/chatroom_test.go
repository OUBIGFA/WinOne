package flashchat

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Ported from chat/ChatRoom.test.ts: real ChatRooms against each other
// over MemoryTransport.

const testRoom = "test-room"

var (
	identitiesOnce sync.Once
	identities     [3]*Identity
)

func identity(t *testing.T, i int) *Identity {
	identitiesOnce.Do(func() {
		for j := range identities {
			id, err := NewIdentity()
			if err != nil {
				panic(err)
			}
			identities[j] = id
		}
	})
	return identities[i]
}

// fakeFiles records the file transfer the rooms ask for.
type fakeFiles struct {
	magnet     string
	mu         sync.Mutex
	rescinded  []string
	all        int
	rescindErr error
	offering   bool
}

func (f *fakeFiles) Offer([]FileHandle, string) (string, error) { return f.magnet, nil }
func (f *fakeFiles) Rescind(m string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rescinded = append(f.rescinded, m)
	err := f.rescindErr
	f.rescindErr = nil
	return err
}
func (f *fakeFiles) RescindAll() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.all++
	return nil
}
func (f *fakeFiles) IsOffering(string) bool { return f.offering }
func (f *fakeFiles) rescindedWith(m string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.rescinded {
		if r == m {
			return true
		}
	}
	return false
}

type client struct {
	room      *ChatRoom
	transport *MemoryTransport
	files     *fakeFiles
	index     int
}

type clientOptions struct {
	password       string
	customUsername string
	target         string
	shareHistory   bool
	limit          int
}

func ids() func() string {
	var n atomic.Int64
	return func() string { return fmt.Sprintf("id-%d", n.Add(1)) }
}

func newClient(t *testing.T, n *MemoryNetwork, i int, o clientOptions) *client {
	peer := fmt.Sprintf("peer-%d", i)
	transport := n.Join(peer)
	files := &fakeFiles{magnet: "magnet:" + peer, offering: true}
	room := NewChatRoom(Options{RoomID: testRoom, UserID: fmt.Sprintf("user-%d", i), CustomUsername: o.customUsername, Identity: identity(t, i),
		Transport: transport, Files: files, Password: o.password, ShareHistoryOnJoin: o.shareHistory, TargetPeerID: o.target,
		TranscriptSizeLimit: o.limit, NewID: ids(), TypingExpiry: 300 * time.Millisecond, Logf: t.Logf})
	t.Cleanup(transport.Leave)
	return &client{room: room, transport: transport, files: files, index: i}
}

// direct opens a direct-message room on a client's transport.
func (c *client) direct(t *testing.T, target string) *ChatRoom {
	return NewChatRoom(Options{RoomID: testRoom, UserID: fmt.Sprintf("user-%d", c.index), Identity: identity(t, c.index), Transport: c.transport,
		Files: c.files, TargetPeerID: target, NewID: ids(), TypingExpiry: 300 * time.Millisecond, Logf: t.Logf})
}

func waitFor(t *testing.T, label string, ok func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !ok() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", label)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// settle lets in-flight traffic drain when there is nothing to wait for.
func settle() { time.Sleep(100 * time.Millisecond) }

func joinAll(t *testing.T, clients ...*client) {
	t.Helper()
	for _, c := range clients {
		c.room.Join()
	}
	waitFor(t, "every client to see the others", func() bool {
		for _, c := range clients {
			if len(c.room.Peers()) != len(clients)-1 {
				return false
			}
		}
		return true
	})
	// The greetings that follow a meeting are still in flight.
	settle()
}

type recorder struct {
	mu     sync.Mutex
	events []Event
}

func record(room *ChatRoom, kind EventKind) *recorder {
	r := &recorder{}
	room.On(kind, func(e Event) {
		r.mu.Lock()
		r.events = append(r.events, e)
		r.mu.Unlock()
	})
	return r
}

func (r *recorder) list() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...)
}

func TestMessagingBetweenTwoClients(t *testing.T) {
	n := NewMemoryNetwork()
	alice, bob := newClient(t, n, 0, clientOptions{}), newClient(t, n, 1, clientOptions{})
	joinAll(t, alice, bob)
	received := record(bob.room, MessageReceived)
	if err := alice.room.SendMessage("hello bob"); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "bob to receive", func() bool { return len(bob.room.MessageLog()) == 1 })
	m := bob.room.MessageLog()[0]
	if m.Text != "hello bob" || m.AuthorID != "user-0" || !m.Received {
		t.Errorf("received %+v", m)
	}
	settle()
	if got := received.list(); len(got) != 1 || got[0].PeerID != "peer-0" {
		t.Errorf("MESSAGE_RECEIVED %+v", got)
	}
	// No echo back to the sender.
	if len(alice.room.MessageLog()) != 1 {
		t.Errorf("alice log %+v", alice.room.MessageLog())
	}
}

func TestOwnMessageIsRecordedThenMarkedDelivered(t *testing.T) {
	alice := newClient(t, NewMemoryNetwork(), 0, clientOptions{})
	alice.room.Join()
	var states []bool
	var sending []bool
	alice.room.On(MessageLogChange, func(Event) {
		log := alice.room.MessageLog()
		states = append(states, len(log) == 1 && log[0].Received)
	})
	alice.room.On(SendingStateChange, func(e Event) { sending = append(sending, e.Sending) })
	if err := alice.room.SendMessage("hi"); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(states, []bool{false, true}) || !reflect.DeepEqual(sending, []bool{true, false}) || alice.room.IsMessageSending() {
		t.Errorf("log states %v sending %v", states, sending)
	}
}

func TestPeersAreVerified(t *testing.T) {
	n := NewMemoryNetwork()
	alice, bob := newClient(t, n, 0, clientOptions{customUsername: "Ada"}), newClient(t, n, 1, clientOptions{customUsername: "Bob"})
	joinAll(t, alice, bob)
	a, b := alice.room.Peers()[0], bob.room.Peers()[0]
	if a.PeerID != "peer-1" || a.UserID != "user-1" || a.CustomUsername != "Bob" || a.Verification != Verified {
		t.Errorf("bob as seen by alice %+v", a)
	}
	if b.PeerID != "peer-0" || b.CustomUsername != "Ada" || b.Verification != Verified {
		t.Errorf("alice as seen by bob %+v", b)
	}
}

func TestForgedSignatureIsUnverified(t *testing.T) {
	n := NewMemoryNetwork()
	alice := newClient(t, n, 0, clientOptions{})
	alice.room.Join()
	impostor := n.Join("peer-9")
	impostor.OnPeerJoin(HookNewPeer, func(string) {})
	verifications := record(alice.room, PeerVerification)
	waitFor(t, "the impostor to connect", func() bool { return len(impostor.Peers()) == 1 })
	// Advertise key 1, sign with key 2.
	proof, _ := identity(t, 2).Sign(testRoom, "user-9")
	_ = impostor.MakeAction(ActionName(NamespaceGroup, ActionPeerMetadata)).Send(UserMetadata{UserID: "user-9", CustomUsername: "Impostor",
		PublicKeyString: identity(t, 1).PublicKey(), IdentitySignatureBase64: proof}, "")
	waitFor(t, "the verification", func() bool { return len(verifications.list()) == 1 })
	if verifications.list()[0].Verification != Unverified || alice.room.Peers()[0].Verification != Unverified {
		t.Errorf("verification %+v", verifications.list())
	}
	// An unparseable key is ignored altogether.
	_ = impostor.MakeAction(ActionName(NamespaceGroup, ActionPeerMetadata)).Send(UserMetadata{UserID: "user-9", PublicKeyString: "nope"}, "")
	settle()
	if len(verifications.list()) != 1 {
		t.Error("metadata with an unparseable key was used")
	}
}

func TestRenameNamesWhatTheUserSaw(t *testing.T) {
	n := NewMemoryNetwork()
	alice, bob := newClient(t, n, 0, clientOptions{}), newClient(t, n, 1, clientOptions{customUsername: "Bob"})
	joinAll(t, alice, bob)
	renames := record(alice.room, PeerRename)
	bob.room.SetCustomUsername("Roberta")
	waitFor(t, "the rename", func() bool { return len(renames.list()) == 1 })
	e := renames.list()[0]
	if e.PeerID != "peer-1" || e.PreviousName != "Bob ("+PeerName("user-1")+")" || e.DisplayName != "Roberta ("+PeerName("user-1")+")" {
		t.Errorf("rename %+v", e)
	}
	// A rename that changes the user id too names the peer the user saw.
	proof, _ := identity(t, 1).Sign(testRoom, "user-1-reimported")
	_ = bob.transport.MakeAction(ActionName(NamespaceGroup, ActionPeerMetadata)).Send(UserMetadata{UserID: "user-1-reimported",
		PublicKeyString: identity(t, 1).PublicKey(), IdentitySignatureBase64: proof}, "")
	waitFor(t, "the second rename", func() bool { return len(renames.list()) == 2 })
	if e := renames.list()[1]; e.PreviousName != "Roberta ("+PeerName("user-1")+")" || e.DisplayName != PeerName("user-1-reimported") {
		t.Errorf("rename %+v", e)
	}
}

func TestLeavingPeerIsRemoved(t *testing.T) {
	n := NewMemoryNetwork()
	alice, bob := newClient(t, n, 0, clientOptions{}), newClient(t, n, 1, clientOptions{})
	joinAll(t, alice, bob)
	leaves := record(alice.room, PeerLeave)
	bob.transport.Leave()
	waitFor(t, "the departure", func() bool { return len(alice.room.Peers()) == 0 && len(leaves.list()) == 1 })
	if e := leaves.list()[0]; e.PeerID != "peer-1" || e.Peer == nil || e.Peer.UserID != "user-1" {
		t.Errorf("leave %+v", e)
	}
}

func TestTypingIsAnnouncedOnceAndExpires(t *testing.T) {
	n := NewMemoryNetwork()
	alice, bob := newClient(t, n, 0, clientOptions{}), newClient(t, n, 1, clientOptions{})
	joinAll(t, alice, bob)
	statuses := record(bob.room, TypingStatusChange)
	alice.room.NotifyTyping()
	waitFor(t, "the announcement", func() bool { return len(statuses.list()) == 1 })
	alice.room.NotifyTyping()
	alice.room.NotifyTyping()
	if !alice.room.IsTyping() || len(statuses.list()) != 1 || !bob.room.Peers()[0].TypingGroup {
		t.Errorf("typing %v statuses %d", alice.room.IsTyping(), len(statuses.list()))
	}
	waitFor(t, "the expiry", func() bool { return len(statuses.list()) == 2 })
	if statuses.list()[1].IsTyping || alice.room.IsTyping() {
		t.Error("typing did not expire")
	}
	// A message clears the sender's typing flag.
	alice.room.NotifyTyping()
	waitFor(t, "bob to see alice typing", func() bool { return bob.room.Peers()[0].TypingGroup })
	_ = alice.room.SendMessage("done typing")
	waitFor(t, "the flag to clear", func() bool { return !bob.room.Peers()[0].TypingGroup })
}

func TestHistoryIsSharedWithLaterMembersWhenEnabled(t *testing.T) {
	n := NewMemoryNetwork()
	o := clientOptions{password: "room-secret", shareHistory: true}
	alice := newClient(t, n, 0, o)
	alice.room.Join()
	_ = alice.room.SendMessage("before anyone arrived")
	if !alice.room.MessageLog()[0].Received {
		t.Fatal("own message not delivered with nobody there")
	}
	bob := newClient(t, n, 1, o)
	bob.room.Join()
	waitFor(t, "the backfill", func() bool { return len(bob.room.MessageLog()) == 1 })
	if bob.room.MessageLog()[0].Text != "before anyone arrived" {
		t.Errorf("backfill %+v", bob.room.MessageLog())
	}
}

func TestSharedHistoryMergesWithoutDuplicates(t *testing.T) {
	n := NewMemoryNetwork()
	alice := newClient(t, n, 0, clientOptions{password: "room-secret", shareHistory: true})
	alice.room.Join()
	_ = alice.room.SendMessage("mine")
	sender := n.Join("peer-9")
	sender.OnPeerJoin(HookNewPeer, func(string) {})
	waitFor(t, "the sender", func() bool { return len(sender.Peers()) == 1 })
	history := []Message{{ID: "earlier", Text: "earlier", AuthorID: "user-9", Received: true}}
	h := sender.MakeAction(ActionName(NamespaceGroup, ActionMessageTranscript))
	_ = h.Send(history, "")
	_ = h.Send(history, "")
	waitFor(t, "the merge", func() bool { return len(alice.room.MessageLog()) == 2 })
	settle()
	log := alice.room.MessageLog()
	if len(log) != 2 || log[0].Text != "earlier" || log[1].Text != "mine" {
		t.Errorf("log %+v", log)
	}
}

func TestPublicRoomBackfillsAndPrivateRoomWithholds(t *testing.T) {
	for _, password := range []string{"", "hunter2"} {
		n := NewMemoryNetwork()
		o := clientOptions{password: password}
		alice, bob := newClient(t, n, 0, o), newClient(t, n, 1, o)
		joinAll(t, alice, bob)
		_ = alice.room.SendMessage("said before carol arrived")
		settle()
		carol := newClient(t, n, 2, o)
		carol.room.Join()
		waitFor(t, "carol to meet them", func() bool { return len(carol.room.Peers()) == 2 })
		settle()
		want := 1
		if password != "" {
			want = 0
		}
		if got := len(carol.room.MessageLog()); got != want {
			t.Errorf("password %q: carol has %d messages", password, got)
		}
	}
}

func TestBackfillDoesNotOverwriteAnExistingLog(t *testing.T) {
	n := NewMemoryNetwork()
	alice := newClient(t, n, 0, clientOptions{})
	alice.room.Join()
	_ = alice.room.SendMessage("mine")
	impostor := n.Join("peer-9")
	impostor.OnPeerJoin(HookNewPeer, func(string) {})
	waitFor(t, "the impostor", func() bool { return len(impostor.Peers()) == 1 })
	_ = impostor.MakeAction(ActionName(NamespaceGroup, ActionMessageTranscript)).Send([]Message{{ID: "other", Text: "theirs", AuthorID: "user-9", Received: true}}, "")
	settle()
	if log := alice.room.MessageLog(); len(log) != 1 || log[0].Text != "mine" {
		t.Errorf("log %+v", log)
	}
}

func TestPeersConnectedBeforeJoinStillMeet(t *testing.T) {
	n := NewMemoryNetwork()
	alice, bob := newClient(t, n, 0, clientOptions{}), newClient(t, n, 1, clientOptions{})
	alice.transport.OnPeerJoin("AUDIO", func(string) {})
	bob.transport.OnPeerJoin("AUDIO", func(string) {})
	waitFor(t, "the transports", func() bool { return len(alice.transport.Peers()) == 1 })
	_ = alice.room.SendMessage("") // not joined: no sender, nothing recorded twice
	joinAll(t, alice, bob)
	if alice.room.Peers()[0].Verification != Verified {
		t.Error("not verified")
	}
}

func TestTranscriptLimitEvictsAndStopsSeeding(t *testing.T) {
	c := newClient(t, NewMemoryNetwork(), 0, clientOptions{limit: 3})
	c.files.magnet = "magnet:evicted"
	c.room.Join()
	if err := c.room.SendInlineMedia([]FileHandle{{Name: "a.png", Type: "image/png", Size: 1}}); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"one", "two", "three"} {
		_ = c.room.SendMessage(text)
	}
	var texts []string
	for _, m := range c.room.MessageLog() {
		texts = append(texts, m.Text)
	}
	if !reflect.DeepEqual(texts, []string{"one", "two", "three"}) {
		t.Errorf("log %v", texts)
	}
	waitFor(t, "the evicted media to stop", func() bool { return c.files.rescindedWith("magnet:evicted") })
}

func TestFileOffersPropagateAndClear(t *testing.T) {
	n := NewMemoryNetwork()
	alice, bob := newClient(t, n, 0, clientOptions{}), newClient(t, n, 1, clientOptions{})
	joinAll(t, alice, bob)
	changes := record(bob.room, FileOfferChange)
	if _, err := alice.room.OfferFiles([]FileHandle{{Name: "doc.pdf", Type: "application/pdf", Size: 10}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the offer", func() bool { return len(bob.room.FileOffers()) == 1 })
	if got := bob.room.FileOffers()["peer-0"]; got != (FileOfferMetadata{MagnetURI: "magnet:peer-0"}) || bob.room.Peers()[0].OfferedFileID != "magnet:peer-0" {
		t.Errorf("offer %+v", got)
	}
	if len(changes.list()) != 1 || alice.room.SelfFileOffer() != "magnet:peer-0" {
		t.Errorf("changes %d self %q", len(changes.list()), alice.room.SelfFileOffer())
	}
	alice.room.RescindFileOffer()
	waitFor(t, "the withdrawal", func() bool { return len(bob.room.FileOffers()) == 0 })
	if bob.room.Peers()[0].OfferedFileID != "" || !alice.files.rescindedWith("magnet:peer-0") || alice.room.SelfFileOffer() != "" {
		t.Error("offer not cleared on both sides")
	}
	// An all-inline-media offer stays seeded: the transcript shows it.
	_, _ = alice.room.OfferFiles([]FileHandle{{Name: "a.png", Type: "image/png", Size: 1}})
	alice.files.rescinded = nil
	alice.room.RescindFileOffer()
	if alice.files.rescindedWith("magnet:peer-0") {
		t.Error("inline media was rescinded")
	}
}

func TestStandingOfferReachesLaterPeerAndLeavesWithItsPeer(t *testing.T) {
	n := NewMemoryNetwork()
	alice := newClient(t, n, 0, clientOptions{})
	alice.room.Join()
	_, _ = alice.room.OfferFiles([]FileHandle{{Name: "doc.pdf", Type: "application/pdf", Size: 10}})
	bob := newClient(t, n, 1, clientOptions{})
	// The offer may reach bob twice (alice's broadcast and her greeting),
	// and listeners hear a change after the room's state shows it: count
	// from the start, and only once the arrival has settled.
	changes := record(bob.room, FileOfferChange)
	bob.room.Join()
	waitFor(t, "the standing offer", func() bool {
		return bob.room.FileOffers()["peer-0"].MagnetURI == "magnet:peer-0" && len(changes.list()) > 0
	})
	settle()
	before := len(changes.list())
	alice.transport.Leave()
	waitFor(t, "the offer to go with alice", func() bool {
		return len(bob.room.FileOffers()) == 0 && len(changes.list()) == before+1
	})
	settle()
	if got := len(changes.list()); got != before+1 {
		t.Fatalf("alice's departure changed the offers %d times", got-before)
	}
	if !bob.files.rescindedWith("magnet:peer-0") {
		t.Error("departed offer not rescinded")
	}
}

func TestFailedOfferAnnouncementStopsTheShare(t *testing.T) {
	n := NewMemoryNetwork()
	alice := newClient(t, n, 0, clientOptions{})
	offerName := ActionName(NamespaceGroup, ActionFileOffer)
	var sends atomic.Int32
	allowed := int32(0)
	alice.transport.FailSend = func(name string) error {
		if name == offerName && sends.Add(1) > allowed {
			return errors.New("send failed")
		}
		return nil
	}
	alice.room.Join()
	if _, err := alice.room.OfferFiles([]FileHandle{{Name: "doc.pdf", Type: "application/pdf"}}); err == nil || err.Error() != "send failed" {
		t.Fatalf("offer error %v", err)
	}
	if !alice.files.rescindedWith("magnet:peer-0") || alice.room.SelfFileOffer() != "" {
		t.Error("a share nobody was told about kept running")
	}
	// A rescind that cannot be announced still stops locally, and reports.
	sends.Store(0)
	allowed = 1
	alice.files.rescinded = nil
	if _, err := alice.room.OfferFiles([]FileHandle{{Name: "doc.pdf", Type: "application/pdf"}}); err != nil {
		t.Fatal(err)
	}
	errs := record(alice.room, Error)
	alice.room.RescindFileOffer()
	if alice.room.SelfFileOffer() != "" || !alice.files.rescindedWith("magnet:peer-0") || len(errs.list()) != 1 || errs.list()[0].Context != ContextRescindFileOffer {
		t.Errorf("rescind %q errors %+v", alice.room.SelfFileOffer(), errs.list())
	}
}

func TestStopSeedingFailureIsReported(t *testing.T) {
	n := NewMemoryNetwork()
	alice, bob := newClient(t, n, 0, clientOptions{}), newClient(t, n, 1, clientOptions{})
	joinAll(t, alice, bob)
	_, _ = bob.room.OfferFiles([]FileHandle{{Name: "doc.pdf", Type: "application/pdf"}})
	waitFor(t, "the offer", func() bool { return len(alice.room.FileOffers()) == 1 })
	errs := record(alice.room, Error)
	alice.files.rescindErr = errors.New("rescind failed")
	bob.transport.Leave()
	waitFor(t, "the error", func() bool { return len(errs.list()) == 1 })
	if errs.list()[0].Context != ContextStopSeeding {
		t.Errorf("context %v", errs.list()[0].Context)
	}
}

func TestDirectRoomsKeepToTheirCorrespondent(t *testing.T) {
	n := NewMemoryNetwork()
	alice, bob, carol := newClient(t, n, 0, clientOptions{}), newClient(t, n, 1, clientOptions{}), newClient(t, n, 2, clientOptions{})
	joinAll(t, alice, bob, carol)
	aliceToBob, aliceToCarol := alice.direct(t, "peer-1"), alice.direct(t, "peer-2")
	bobToAlice, carolToAlice := bob.direct(t, "peer-0"), carol.direct(t, "peer-0")
	for _, r := range []*ChatRoom{aliceToBob, aliceToCarol, bobToAlice, carolToAlice} {
		r.Join()
	}
	settle()
	_ = bobToAlice.SendMessage("between you and me")
	bobToAlice.NotifyTyping()
	_, _ = bobToAlice.OfferFiles([]FileHandle{{Name: "secret.pdf", Type: "application/pdf"}})
	waitFor(t, "the direct message", func() bool { return len(aliceToBob.MessageLog()) == 1 && len(aliceToBob.FileOffers()) == 1 })
	settle()
	if len(aliceToCarol.MessageLog()) != 0 || len(aliceToCarol.FileOffers()) != 0 {
		t.Error("bob's conversation leaked into carol's")
	}
	for _, p := range aliceToCarol.Peers() {
		if p.PeerID != "peer-2" || p.TypingDirect {
			t.Errorf("carol's conversation has %+v", p)
		}
	}
	if len(alice.room.MessageLog()) != 0 || len(carolToAlice.MessageLog()) != 0 {
		t.Error("the group or another conversation got the direct message")
	}
	if peers := aliceToBob.Peers(); len(peers) != 1 || peers[0].PeerID != "peer-1" || !peers[0].TypingDirect || peers[0].TypingGroup {
		t.Errorf("conversation peers %+v", peers)
	}
	// A direct conversation's history is not handed over when the other
	// side opens it later.
	_ = aliceToCarol.SendMessage("said before you opened this")
	settle()
	late := carol.direct(t, "peer-0")
	late.Join()
	settle()
	if len(late.MessageLog()) != 0 {
		t.Error("direct history was backfilled")
	}
}

func TestRegistryForwardsDeparturesAndReturns(t *testing.T) {
	n := NewMemoryNetwork()
	alice, bob := newClient(t, n, 0, clientOptions{}), newClient(t, n, 1, clientOptions{})
	joinAll(t, alice, bob)
	registry := NewRegistry()
	registry.SetGroup(alice.room)
	aliceToBob := alice.direct(t, "peer-1")
	registry.SetDirect("peer-1", aliceToBob)
	aliceToBob.Join()
	bob.direct(t, "peer-0").Join()
	waitFor(t, "the correspondent", func() bool { return len(aliceToBob.Peers()) == 1 })
	bob.transport.Leave()
	waitFor(t, "the correspondent to go", func() bool { return len(aliceToBob.Peers()) == 0 })
	// The same peer id comes back.
	again := newClient(t, n, 1, clientOptions{})
	again.room.Join()
	againToAlice := again.direct(t, "peer-0")
	againToAlice.Join()
	waitFor(t, "the correspondent to return", func() bool { return len(aliceToBob.Peers()) == 1 })
	settle()
	againToAlice.NotifyTyping()
	waitFor(t, "its typing", func() bool { return aliceToBob.Peers()[0].TypingDirect })
}

func TestLeave(t *testing.T) {
	n := NewMemoryNetwork()
	alice, bob := newClient(t, n, 0, clientOptions{}), newClient(t, n, 1, clientOptions{})
	joinAll(t, alice, bob)
	dm := alice.direct(t, "peer-1")
	dm.Join()
	dm.Leave()
	if alice.files.all != 0 {
		t.Error("closing a conversation stopped the group's shares")
	}
	bob.room.Leave()
	bob.room.Leave()
	_ = alice.room.SendMessage("anyone there?")
	settle()
	if len(bob.room.MessageLog()) != 0 {
		t.Error("a left room still processes traffic")
	}
	if bob.files.all != 1 {
		t.Errorf("leave stopped seeding %d times", bob.files.all)
	}
}

func TestFailedSendIsReported(t *testing.T) {
	c := newClient(t, NewMemoryNetwork(), 0, clientOptions{})
	c.transport.FailSend = func(name string) error {
		if name == ActionName(NamespaceGroup, ActionMessage) {
			return errors.New("transport is gone")
		}
		return nil
	}
	c.room.Join()
	errs := record(c.room, Error)
	if err := c.room.SendMessage("does not wedge"); err == nil {
		t.Fatal("no error")
	}
	if got := errs.list(); len(got) != 1 || got[0].Context != ContextSendMessage || got[0].Err.Error() != "transport is gone" || c.room.IsMessageSending() {
		t.Errorf("errors %+v", got)
	}
	if c.room.MessageLog()[0].Received {
		t.Error("an undelivered message is marked delivered")
	}
}

func TestMessageWireShape(t *testing.T) {
	data, _ := marshalJSON(unsent(Message{ID: "m1", AuthorID: "u1", Text: "hi <b>", TimeSent: 1730000000000, Received: true, TimeReceived: 5}))
	if string(data) != `{"authorId":"u1","text":"hi <b>","timeSent":1730000000000,"id":"m1"}` {
		t.Errorf("message %s", data)
	}
	data, _ = marshalJSON(Message{ID: "m2", AuthorID: "u1", Media: true, MagnetURI: "tc1", TimeSent: 1, Received: true, TimeReceived: 2})
	if string(data) != `{"authorId":"u1","magnetURI":"tc1","timeSent":1,"id":"m2","timeReceived":2}` {
		t.Errorf("media %s", data)
	}
	var m Message
	if err := json.Unmarshal([]byte(`{"id":"x","authorId":"a","timeSent":3,"magnetURI":"m","timeReceived":4}`), &m); err != nil || !m.Media || !m.Received || m.TimeReceived != 4 {
		t.Errorf("parsed %+v %v", m, err)
	}
	for _, bad := range []string{`{}`, `{"id":1,"authorId":"a","timeSent":1,"text":""}`, `{"id":"x","authorId":"a","timeSent":"1","text":""}`, `[]`} {
		if json.Unmarshal([]byte(bad), &m) == nil {
			t.Errorf("accepted %s", bad)
		}
	}
}

func TestNamesMatchFunAnimalNames(t *testing.T) {
	// Printed by fun-animal-names 0.1.1.
	for id, name := range map[string]string{"": "Adorable Egret", "a": "Adorable Egret",
		"6f9619ff-8b86-d011-b42d-00c04fc964ff": "Adventurous Llama", "0b3c7c5e-1c5a-4a59-9d55-7ad0bc0f6a31": "Delightful Catfish"} {
		if got := PeerName(id); got != name {
			t.Errorf("%q: %q, want %q", id, got, name)
		}
	}
	if DisplayName("Ada", "a") != "Ada (Adorable Egret)" || DisplayName("", "a") != "Adorable Egret" {
		t.Error("display names")
	}
}

func TestIdentityInteroperatesWithWebCrypto(t *testing.T) {
	// A WebCrypto RSASSA-PKCS1-v1_5 key and its signature over
	// "<room>_user-1", printed by vectors.mjs.
	const room = "3f1c2a9e-7b4d-4e7a-9c1f-2b8d6e5a4c3b"
	const key = "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEAwbQ2gAUR6wnIoZz6tsIAzOFE4IXQ1NjTBKY81INFGJzKqtKiQR1WkjdBCS2F4/XxyKtwIiTTKJSeY0+AMw9FpWhH8mM4x/NmtMQ4e69jEvroc6KSm8sxwexelyRlNTQJ3XoMNQN/ZobUthYtigk8SA1mIGxYm3mzWNAgV/vAHN1xICkBTs803k9HEjQkggksEPUMxdIooMP0mFSIn+lgGuAtnkgELuZ5V8aMUYMb5AFV0TWHxLxkMnu6vrA42uYJj92cSi06zT0VLLU1Ll8f5fDVc3sKkDm8MxFkBIEs7hfZVY70dMmp6YCgJJNAy0Y4k4Es2aJTp7dimr0pCj5RZQIDAQAB"
	const signature = "mbDbtp57EuWu389cimMN8nqbCyWm70kNFIlbQHSLPghFHgCSWEfkQGfxBm2Hgs0wJD0hIWbbkkDkRR3//K5gO9YbrCVIblCj1SVAl/m0T9aZrZMMkADKN3T5et/k5zjLJTbxkzpi11FTC8i+rP3KKY4vAyHP90+UF2oGTGhvG2qSW7Ni1R/xP5Fhz7ET0B4LfIduLigU0OKzz1/rCFIxTFVAaYiAB+x1ICVCmVDAFpOeCljOr5/3bUrLuNv0SimKYy2qW9D31+LajfjPZ+BeLeGzYbmYJ3WlX/O5KXeghvAvS44pSLvd0Sk6UmR6xCsR96nc/X2PGXL+SoXSB4DT4A=="
	if state, ok := VerifyIdentity(key, signature, room, "user-1"); !ok || state != Verified {
		t.Errorf("WebCrypto proof: %v %v", state, ok)
	}
	for _, c := range []struct{ room, user string }{{room, "user-2"}, {"other", "user-1"}} {
		if state, ok := VerifyIdentity(key, signature, c.room, c.user); !ok || state != Unverified {
			t.Errorf("replayed proof %v accepted: %v", c, state)
		}
	}
	if state, ok := VerifyIdentity(key, "AAAA"+signature[4:], room, "user-1"); !ok || state != Unverified {
		t.Error("tampered proof accepted")
	}
	if _, ok := VerifyIdentity("not a key", signature, room, "user-1"); ok {
		t.Error("an unparseable key was used")
	}
	own := identity(t, 0)
	proof, _ := own.Sign(room, "me")
	if state, _ := VerifyIdentity(own.PublicKey(), proof, room, "me"); state != Verified {
		t.Error("own proof")
	}
}
