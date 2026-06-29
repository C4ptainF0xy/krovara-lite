package sfu

import (
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
)

var ridRank = map[string]int{"h": 0, "m": 1, "l": 2}

func ridBetter(candidate, current string) bool {
	if current == "" {
		return true
	}
	rc, okc := ridRank[candidate]
	rcur, okcur := ridRank[current]
	if !okc {
		return false
	}
	if !okcur {
		return true
	}
	return rc < rcur
}

const tsSwitchGap = 3000

type simulcastTrack struct {
	peer    *Peer
	codec   webrtc.RTPCodecCapability
	trackID string
	keyOf   func([]byte) bool

	mu          sync.Mutex
	layers      map[string]*webrtc.TrackRemote
	downstreams map[string]*downstream
	live        int
	dead        bool
}

type downstream struct {
	out *webrtc.TrackLocalStaticRTP
	rid string

	started    bool
	curRID     string
	seqOff     uint16
	tsOff      uint32
	lastSeq    uint16
	lastTS     uint32
	waitKey    bool
	needOffset bool
}

func newSimulcastTrack(p *Peer, remote *webrtc.TrackRemote) *simulcastTrack {
	return &simulcastTrack{
		peer:        p,
		codec:       remote.Codec().RTPCodecCapability,
		trackID:     remote.ID(),
		keyOf:       keyframeDetector(remote.Codec().MimeType),
		layers:      map[string]*webrtc.TrackRemote{},
		downstreams: map[string]*downstream{},
	}
}

func (p *Peer) onSimulcastLayer(remote *webrtc.TrackRemote) error {
	rid := remote.RID()
	trackID := remote.ID()

	p.pubMu.Lock()
	st, exists := p.simulcast[trackID]
	if !exists {
		st = newSimulcastTrack(p, remote)
		p.simulcast[trackID] = st
	}
	p.pubMu.Unlock()

	st.mu.Lock()
	st.layers[rid] = remote
	st.live++

	for _, d := range st.downstreams {
		if st.layers[d.rid] == nil {
			d.rid = st.bestRIDLocked()
		}
	}
	st.mu.Unlock()

	if !exists {
		p.fanoutSimulcast(st)
		go st.nudgeKeyframes()
	}

	st.pipeLayer(rid, remote)

	st.mu.Lock()
	st.live--
	last := st.live == 0
	st.mu.Unlock()
	if last {
		p.unpublishSimulcast(st)
	}
	return nil
}

func (p *Peer) unpublishSimulcast(st *simulcastTrack) {
	p.pubMu.Lock()
	delete(p.simulcast, st.trackID)
	p.pubMu.Unlock()

	st.mu.Lock()
	if st.dead {
		st.mu.Unlock()
		return
	}
	st.dead = true
	type sub struct {
		id  string
		out *webrtc.TrackLocalStaticRTP
	}
	subs := make([]sub, 0, len(st.downstreams))
	for id, d := range st.downstreams {
		subs = append(subs, sub{id, d.out})
	}
	st.downstreams = map[string]*downstream{}
	st.mu.Unlock()

	r := p.room.Load()
	if r == nil {
		return
	}
	for _, s := range subs {
		if peer := r.Peer(s.id); peer != nil {
			peer.unsubscribeTrack(s.out)
		}
	}
}

func (st *simulcastTrack) bestRIDLocked() string {
	best := ""
	for rid := range st.layers {
		if ridBetter(rid, best) {
			best = rid
		}
	}
	return best
}

func (st *simulcastTrack) addDownstream(subscriberID string, out *webrtc.TrackLocalStaticRTP) {
	st.mu.Lock()
	rid := st.bestRIDLocked()
	st.downstreams[subscriberID] = &downstream{out: out, rid: rid}
	remote := st.layers[rid]
	st.mu.Unlock()
	if remote != nil {
		st.requestPLI(remote)
	}
}

func (st *simulcastTrack) removeDownstream(subscriberID string) {
	st.mu.Lock()
	delete(st.downstreams, subscriberID)
	st.mu.Unlock()
}

func (st *simulcastTrack) setLayer(subscriberID, rid string) {
	st.mu.Lock()
	d, ok := st.downstreams[subscriberID]
	if !ok || st.layers[rid] == nil || d.rid == rid {
		st.mu.Unlock()
		return
	}
	d.rid = rid
	remote := st.layers[rid]
	st.mu.Unlock()
	st.requestPLI(remote)
}

func (st *simulcastTrack) pipeLayer(rid string, remote *webrtc.TrackRemote) {
	for {
		pkt, _, err := remote.ReadRTP()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				st.peer.log.Debug("simulcast layer read", "rid", rid, "err", err)
			}
			return
		}
		keyframe := st.keyOf(pkt.Payload)

		st.mu.Lock()
		for _, d := range st.downstreams {
			if d.rid != rid {
				continue
			}
			st.forward(d, rid, pkt, keyframe)
		}
		st.mu.Unlock()
	}
}

func (st *simulcastTrack) forward(d *downstream, rid string, pkt *rtp.Packet, keyframe bool) {
	out, ok := d.rewrite(rid, pkt, keyframe)
	if !ok {
		return
	}
	if err := d.out.WriteRTP(out); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		st.peer.log.Debug("simulcast downstream write", "rid", rid, "err", err)
	}
}

func (d *downstream) rewrite(rid string, pkt *rtp.Packet, keyframe bool) (*rtp.Packet, bool) {
	if d.curRID != rid {
		d.curRID = rid
		if d.started {

			d.waitKey = true
			d.needOffset = true
		}
	}
	if d.waitKey {
		if !keyframe {
			return nil, false
		}
		d.waitKey = false
	}
	if d.needOffset {
		d.seqOff = d.lastSeq + 1 - pkt.SequenceNumber
		d.tsOff = d.lastTS + tsSwitchGap - pkt.Timestamp
		d.needOffset = false
	}

	outSeq := pkt.SequenceNumber + d.seqOff
	outTS := pkt.Timestamp + d.tsOff
	d.lastSeq = outSeq
	d.lastTS = outTS
	d.started = true

	out := *pkt
	out.SequenceNumber = outSeq
	out.Timestamp = outTS
	return &out, true
}

func (st *simulcastTrack) nudgeKeyframes() {
	t := time.NewTicker(3 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-st.peer.closed:
			return
		case <-t.C:
			st.mu.Lock()
			dead := st.dead
			inUse := map[string]*webrtc.TrackRemote{}
			for _, d := range st.downstreams {
				if r := st.layers[d.rid]; r != nil {
					inUse[d.rid] = r
				}
			}
			st.mu.Unlock()
			if dead {
				return
			}
			for _, remote := range inUse {
				st.requestPLI(remote)
			}
		}
	}
}

func (st *simulcastTrack) requestPLI(remote *webrtc.TrackRemote) {
	_ = st.peer.pc.WriteRTCP([]rtcp.Packet{
		&rtcp.PictureLossIndication{MediaSSRC: uint32(remote.SSRC())},
	})
}

func keyframeDetector(mimeType string) func([]byte) bool {
	switch strings.ToLower(mimeType) {
	case strings.ToLower(webrtc.MimeTypeVP8):
		return vp8Keyframe
	case strings.ToLower(webrtc.MimeTypeH264):
		return h264Keyframe
	default:
		return func([]byte) bool { return true }
	}
}

func vp8Keyframe(payload []byte) bool {
	var vp8 codecs.VP8Packet
	body, err := vp8.Unmarshal(payload)
	if err != nil || len(body) == 0 {
		return false
	}

	if vp8.S != 1 || vp8.PID != 0 {
		return false
	}

	return body[0]&0x01 == 0
}

func h264Keyframe(payload []byte) bool {
	if len(payload) < 1 {
		return false
	}
	switch nalType := payload[0] & 0x1f; nalType {
	case 5, 7, 8:
		return true
	case 24:
		off := 1
		for off+2 <= len(payload) {
			size := int(payload[off])<<8 | int(payload[off+1])
			off += 2
			if size == 0 || off+size > len(payload) {
				break
			}
			if t := payload[off] & 0x1f; t == 5 || t == 7 {
				return true
			}
			off += size
		}
	case 28, 29:
		if len(payload) < 2 {
			return false
		}
		start := payload[1]&0x80 != 0
		t := payload[1] & 0x1f
		return start && (t == 5 || t == 7)
	}
	return false
}
