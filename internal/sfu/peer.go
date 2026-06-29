package sfu

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"
	"github.com/pion/rtcp"
	"github.com/pion/sdp/v3"
	"github.com/pion/webrtc/v4"
)

type Signal struct {
	Type          string  `json:"type"`
	Room          string  `json:"room,omitempty"`
	SDP           string  `json:"sdp,omitempty"`
	Candidate     string  `json:"candidate,omitempty"`
	SDPMid        *string `json:"sdpMid,omitempty"`
	SDPMLineIndex *uint16 `json:"sdpMLineIndex,omitempty"`
	PeerID        string  `json:"peer_id,omitempty"`
	RID           string  `json:"rid,omitempty"`
}

type PeerConfig struct {
	ID         string
	UserID     string
	WS         *websocket.Conn
	WebRTC     webrtc.Configuration
	Log        *slog.Logger
	ReadBuffer int
}

type Peer struct {
	id     string
	userID string
	ws     *websocket.Conn
	pc     *webrtc.PeerConnection
	api    *webrtc.API
	log    *slog.Logger

	room atomic.Pointer[Room]

	pubMu sync.RWMutex

	published []*webrtc.TrackLocalStaticRTP

	simulcast map[string]*simulcastTrack

	subMu  sync.Mutex
	subbed map[string][]*webrtc.RTPSender

	sendMu sync.Mutex

	negotiateMu  sync.Mutex
	negotiating  bool
	renegPending bool

	makingOffer atomic.Bool

	closeOnce sync.Once
	closed    chan struct{}
}

func NewPeer(cfg PeerConfig) (*Peer, error) {
	log := cfg.Log
	if log == nil {
		log = slog.Default()
	}
	log = log.With("peer_id", cfg.ID, "user_id", cfg.UserID)

	me := &webrtc.MediaEngine{}
	if err := me.RegisterDefaultCodecs(); err != nil {
		return nil, fmt.Errorf("register codecs: %w", err)
	}

	exts := []struct {
		uri  string
		kind webrtc.RTPCodecType
	}{
		{sdp.SDESMidURI, webrtc.RTPCodecTypeAudio},
		{sdp.SDESMidURI, webrtc.RTPCodecTypeVideo},
		{sdp.SDESRTPStreamIDURI, webrtc.RTPCodecTypeVideo},
		{sdp.SDESRepairRTPStreamIDURI, webrtc.RTPCodecTypeVideo},
	}
	for _, e := range exts {
		if err := me.RegisterHeaderExtension(
			webrtc.RTPHeaderExtensionCapability{URI: e.uri}, e.kind,
		); err != nil {
			return nil, fmt.Errorf("register header ext %s: %w", e.uri, err)
		}
	}
	api := webrtc.NewAPI(webrtc.WithMediaEngine(me))

	pc, err := api.NewPeerConnection(cfg.WebRTC)
	if err != nil {
		return nil, fmt.Errorf("new peer connection: %w", err)
	}

	limit := cfg.ReadBuffer
	if limit == 0 {
		limit = 1 << 20
	}
	cfg.WS.SetReadLimit(int64(limit))

	p := &Peer{
		id:        cfg.ID,
		userID:    cfg.UserID,
		ws:        cfg.WS,
		pc:        pc,
		api:       api,
		log:       log,
		subbed:    map[string][]*webrtc.RTPSender{},
		simulcast: map[string]*simulcastTrack{},
		closed:    make(chan struct{}),
	}

	pc.OnICECandidate(func(c *webrtc.ICECandidate) {
		if c == nil {
			return
		}
		init := c.ToJSON()

		if err := p.sendSignal(Signal{
			Type:          "candidate",
			Candidate:     init.Candidate,
			SDPMid:        init.SDPMid,
			SDPMLineIndex: init.SDPMLineIndex,
		}); err != nil {
			log.Debug("send candidate", "err", err)
		}
	})

	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Debug("pc state", "state", s.String())
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed {
			p.Close()
		}
	})

	pc.OnTrack(func(remote *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		if err := p.onRemoteTrack(remote); err != nil {
			log.Warn("on remote track", "err", err)
		}
	})

	return p, nil
}

func (p *Peer) ID() string { return p.id }

func (p *Peer) Published() []*webrtc.TrackLocalStaticRTP {
	p.pubMu.RLock()
	defer p.pubMu.RUnlock()
	out := make([]*webrtc.TrackLocalStaticRTP, len(p.published))
	copy(out, p.published)
	return out
}

func (p *Peer) Subscribe(sourcePeerID string, track *webrtc.TrackLocalStaticRTP) error {
	p.log.Debug("subscribe", "from", sourcePeerID, "track", track.ID())
	sender, err := p.pc.AddTrack(track)
	if err != nil {
		return fmt.Errorf("add track: %w", err)
	}

	go drainSenderRTCP(sender)

	p.subMu.Lock()
	p.subbed[sourcePeerID] = append(p.subbed[sourcePeerID], sender)
	p.subMu.Unlock()

	return p.renegotiate()
}

func (p *Peer) UnsubscribeFrom(sourcePeerID string) {
	p.subMu.Lock()
	senders := p.subbed[sourcePeerID]
	delete(p.subbed, sourcePeerID)
	p.subMu.Unlock()

	if len(senders) == 0 {
		return
	}
	for _, s := range senders {
		if err := p.pc.RemoveTrack(s); err != nil {
			p.log.Debug("remove track", "err", err)
		}
	}
	if err := p.renegotiate(); err != nil {
		p.log.Debug("renegotiate after unsubscribe", "err", err)
	}
}

func (p *Peer) onRemoteTrack(remote *webrtc.TrackRemote) error {
	p.log.Debug("on track", "kind", remote.Kind().String(), "rid", remote.RID(), "id", remote.ID())
	if remote.RID() != "" {
		return p.onSimulcastLayer(remote)
	}
	return p.onSimpleTrack(remote)
}

func (p *Peer) onSimpleTrack(remote *webrtc.TrackRemote) error {
	local, err := webrtc.NewTrackLocalStaticRTP(
		remote.Codec().RTPCodecCapability,
		remote.ID(),
		p.id,
	)
	if err != nil {
		return fmt.Errorf("new local track: %w", err)
	}

	p.pubMu.Lock()
	p.published = append(p.published, local)
	p.pubMu.Unlock()

	go p.forwardPLI(remote)
	p.fanout(local)

	go func() {
		buf := make([]byte, 1500)
		for {
			n, _, err := remote.Read(buf)
			if err != nil {
				if !errors.Is(err, io.EOF) {
					p.log.Debug("remote read", "err", err)
				}
				break
			}
			if _, err := local.Write(buf[:n]); err != nil && !errors.Is(err, io.ErrClosedPipe) {
				p.log.Debug("local write", "err", err)
			}
		}
		p.unpublishSimple(local)
	}()

	return nil
}

func (p *Peer) unpublishSimple(local *webrtc.TrackLocalStaticRTP) {
	p.pubMu.Lock()
	kept := p.published[:0]
	for _, t := range p.published {
		if t != local {
			kept = append(kept, t)
		}
	}
	p.published = kept
	p.pubMu.Unlock()

	if r := p.room.Load(); r != nil {
		for _, other := range r.Others(p.id) {
			other.unsubscribeTrack(local)
		}
	}
}

func (p *Peer) unsubscribeTrack(local webrtc.TrackLocal) {
	var target *webrtc.RTPSender
	for _, s := range p.pc.GetSenders() {
		if s.Track() == local {
			target = s
			break
		}
	}
	if target == nil {
		return
	}
	if err := p.pc.RemoveTrack(target); err != nil {
		p.log.Debug("remove track", "err", err)
		return
	}

	src := local.StreamID()
	p.subMu.Lock()
	senders := p.subbed[src]
	filtered := senders[:0]
	for _, s := range senders {
		if s != target {
			filtered = append(filtered, s)
		}
	}
	p.subbed[src] = filtered
	p.subMu.Unlock()

	if err := p.renegotiate(); err != nil {
		p.log.Debug("renegotiate after unsubscribe track", "err", err)
	}
}

func (p *Peer) fanout(local *webrtc.TrackLocalStaticRTP) {
	r := p.room.Load()
	if r == nil {
		return
	}
	for _, other := range r.Others(p.id) {
		if err := other.Subscribe(p.id, local); err != nil {
			p.log.Warn("subscribe peer", "to", other.id, "err", err)
		}
	}
}

func (p *Peer) fanoutSimulcast(st *simulcastTrack) {
	r := p.room.Load()
	if r == nil {
		return
	}
	for _, other := range r.Others(p.id) {
		if err := other.SubscribeSimulcast(p.id, st); err != nil {
			p.log.Warn("subscribe simulcast", "to", other.id, "err", err)
		}
	}
}

func (p *Peer) addSimulcastDownstream(sourcePeerID string, st *simulcastTrack) error {
	out, err := webrtc.NewTrackLocalStaticRTP(st.codec, st.trackID, sourcePeerID)
	if err != nil {
		return fmt.Errorf("new simulcast out track: %w", err)
	}
	sender, err := p.pc.AddTrack(out)
	if err != nil {
		return fmt.Errorf("add simulcast track: %w", err)
	}
	go drainSenderRTCP(sender)

	p.subMu.Lock()
	p.subbed[sourcePeerID] = append(p.subbed[sourcePeerID], sender)
	p.subMu.Unlock()

	st.addDownstream(p.id, out)
	return nil
}

func (p *Peer) SubscribeSimulcast(sourcePeerID string, st *simulcastTrack) error {
	if err := p.addSimulcastDownstream(sourcePeerID, st); err != nil {
		return err
	}
	return p.renegotiate()
}

func (p *Peer) publishedSimulcast() []*simulcastTrack {
	p.pubMu.RLock()
	defer p.pubMu.RUnlock()
	out := make([]*simulcastTrack, 0, len(p.simulcast))
	for _, st := range p.simulcast {
		out = append(out, st)
	}
	return out
}

func (p *Peer) dropDownstream(subscriberID string) {
	for _, st := range p.publishedSimulcast() {
		st.removeDownstream(subscriberID)
	}
}

func (p *Peer) setSubscriberLayer(subscriberID, rid string) {
	for _, st := range p.publishedSimulcast() {
		st.setLayer(subscriberID, rid)
	}
}

func (p *Peer) forwardPLI(remote *webrtc.TrackRemote) {
	if remote.Kind() != webrtc.RTPCodecTypeVideo {
		return
	}

	ssrc := uint32(remote.SSRC())
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-p.closed:
			return
		case <-t.C:
			_ = p.pc.WriteRTCP([]rtcp.Packet{&rtcp.PictureLossIndication{MediaSSRC: ssrc}})
		}
	}
}

func drainSenderRTCP(s *webrtc.RTPSender) {
	buf := make([]byte, 1500)
	for {
		if _, _, err := s.Read(buf); err != nil {
			return
		}
	}
}

func (p *Peer) Run(ctx context.Context, h *Hub) error {
	defer p.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.closed:
			return nil
		default:
		}

		_, data, err := p.ws.Read(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			st := websocket.CloseStatus(err)
			if st == websocket.StatusNormalClosure || st == websocket.StatusGoingAway {
				return nil
			}
			return fmt.Errorf("ws read: %w", err)
		}

		var sig Signal
		if err := json.Unmarshal(data, &sig); err != nil {
			p.log.Warn("bad signal", "err", err)
			continue
		}

		if err := p.dispatch(sig, h); err != nil {
			p.log.Warn("dispatch", "type", sig.Type, "err", err)
		}
	}
}

func (p *Peer) dispatch(sig Signal, h *Hub) error {
	switch sig.Type {
	case "join":
		return p.handleJoin(sig, h)
	case "offer":
		return p.handleOffer(sig)
	case "answer":
		return p.handleAnswer(sig)
	case "candidate":
		return p.handleCandidate(sig)
	case "set-layer":
		return p.handleSetLayer(sig)
	case "leave":
		p.Close()
		return nil
	default:
		return fmt.Errorf("unknown signal type %q", sig.Type)
	}
}

func (p *Peer) handleJoin(sig Signal, h *Hub) error {
	if sig.Room == "" {
		return errors.New("join: missing room")
	}
	if p.room.Load() != nil {
		return errors.New("join: already in a room")
	}

	r := h.GetOrCreate(sig.Room)

	for _, other := range r.Peers() {
		if other.userID == p.userID && other.id != p.id {
			p.log.Info("evicting older session for same user", "old_peer", other.id)
			other.Close()
		}
	}

	existing := r.Join(p)
	p.room.Store(r)

	for _, other := range existing {
		for _, t := range other.Published() {
			sender, err := p.pc.AddTrack(t)
			if err != nil {
				p.log.Warn("pre-subscribe", "to", other.id, "err", err)
				continue
			}
			go drainSenderRTCP(sender)
			p.subMu.Lock()
			p.subbed[other.id] = append(p.subbed[other.id], sender)
			p.subMu.Unlock()
		}
		for _, st := range other.publishedSimulcast() {
			if err := p.addSimulcastDownstream(other.id, st); err != nil {
				p.log.Warn("pre-subscribe simulcast", "to", other.id, "err", err)
			}
		}
	}

	for _, other := range existing {
		_ = other.sendSignal(Signal{Type: "peer-joined", PeerID: p.id})
	}

	return nil
}

func (p *Peer) handleOffer(sig Signal) error {
	if sig.SDP == "" {
		return errors.New("offer: missing sdp")
	}

	collision := p.makingOffer.Load() || p.pc.SignalingState() != webrtc.SignalingStateStable
	if collision {
		p.log.Debug("offer collision — ignored (impolite)", "state", p.pc.SignalingState().String())
		return nil
	}

	if err := p.pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sig.SDP,
	}); err != nil {
		return fmt.Errorf("set remote offer: %w", err)
	}
	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return fmt.Errorf("create answer: %w", err)
	}
	if err := p.pc.SetLocalDescription(answer); err != nil {
		return fmt.Errorf("set local answer: %w", err)
	}
	return p.sendSignal(Signal{Type: "answer", SDP: answer.SDP})
}

func (p *Peer) handleAnswer(sig Signal) error {
	if sig.SDP == "" {
		return errors.New("answer: missing sdp")
	}
	if err := p.pc.SetRemoteDescription(webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sig.SDP,
	}); err != nil {
		p.log.Warn("set remote answer failed", "err", err)
		return fmt.Errorf("set remote answer: %w", err)
	}

	p.negotiateMu.Lock()
	pending := p.renegPending
	p.renegPending = false
	if !pending {
		p.negotiating = false
	}
	p.negotiateMu.Unlock()
	p.log.Debug("answer applied", "pending", pending)
	if pending {
		return p.doOffer()
	}
	return nil
}

func (p *Peer) handleSetLayer(sig Signal) error {
	if sig.PeerID == "" || sig.RID == "" {
		return errors.New("set-layer: missing peer_id or rid")
	}
	r := p.room.Load()
	if r == nil {
		return nil
	}
	src := r.Peer(sig.PeerID)
	if src == nil {
		return nil
	}
	src.setSubscriberLayer(p.id, sig.RID)
	return nil
}

func (p *Peer) handleCandidate(sig Signal) error {
	if sig.Candidate == "" {
		return nil
	}
	return p.pc.AddICECandidate(webrtc.ICECandidateInit{
		Candidate:     sig.Candidate,
		SDPMid:        sig.SDPMid,
		SDPMLineIndex: sig.SDPMLineIndex,
	})
}

func (p *Peer) renegotiate() error {
	p.negotiateMu.Lock()
	if p.negotiating {
		p.renegPending = true
		p.negotiateMu.Unlock()
		return nil
	}
	p.negotiating = true
	p.negotiateMu.Unlock()
	return p.doOffer()
}

func (p *Peer) doOffer() error {
	p.makingOffer.Store(true)
	offer, err := p.pc.CreateOffer(nil)
	if err == nil {
		err = p.pc.SetLocalDescription(offer)
	}
	p.makingOffer.Store(false)
	if err != nil {
		p.negotiateMu.Lock()
		p.negotiating = false
		p.renegPending = false
		p.negotiateMu.Unlock()
		p.log.Warn("doOffer failed", "err", err)
		return fmt.Errorf("renegotiate: %w", err)
	}
	p.log.Debug("sent offer (renegotiation)")
	return p.sendSignal(Signal{Type: "offer", SDP: offer.SDP})
}

func (p *Peer) sendSignal(s Signal) error {
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	p.sendMu.Lock()
	defer p.sendMu.Unlock()
	select {
	case <-p.closed:
		return errors.New("peer closed")
	default:
	}
	return p.ws.Write(context.Background(), websocket.MessageText, data)
}

func (p *Peer) Close() {
	p.closeOnce.Do(func() {
		close(p.closed)
		if r := p.room.Load(); r != nil {
			r.Leave(p.id)
			for _, other := range r.Peers() {

				other.UnsubscribeFrom(p.id)
				other.dropDownstream(p.id)
				_ = other.sendSignal(Signal{Type: "peer-left", PeerID: p.id})
			}
		}
		_ = p.pc.Close()
		_ = p.ws.Close(websocket.StatusNormalClosure, "bye")
	})
}
