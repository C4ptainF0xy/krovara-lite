package sfu

import (
	"testing"

	"github.com/pion/rtp"
	"github.com/stretchr/testify/require"
)

func pkt(seq uint16, ts uint32) *rtp.Packet {
	return &rtp.Packet{Header: rtp.Header{SequenceNumber: seq, Timestamp: ts}}
}

func TestRewriteSameLayerPassthrough(t *testing.T) {
	d := &downstream{rid: "h"}
	for i := range 5 {
		seq := uint16(1000 + i)
		ts := uint32(90000 + i*3000)
		out, ok := d.rewrite("h", pkt(seq, ts), false)
		require.True(t, ok)
		require.Equal(t, seq, out.SequenceNumber)
		require.Equal(t, ts, out.Timestamp)
	}
}

func TestRewriteLayerSwitchContinuity(t *testing.T) {
	d := &downstream{rid: "h"}

	var lastSeq uint16
	var lastTS uint32
	for i := range 3 {
		out, ok := d.rewrite("h", pkt(uint16(500+i), uint32(10000+i*3000)), false)
		require.True(t, ok)
		lastSeq, lastTS = out.SequenceNumber, out.Timestamp
	}

	_, ok := d.rewrite("l", pkt(60000, 7777), false)
	require.False(t, ok, "inter-frame after a switch must be dropped")

	out, ok := d.rewrite("l", pkt(60001, 8888), true)
	require.True(t, ok)
	require.Equal(t, lastSeq+1, out.SequenceNumber, "sequence stays +1 across the switch")
	require.Equal(t, lastTS+tsSwitchGap, out.Timestamp, "timestamp jumps forward by one gap")

	out2, ok := d.rewrite("l", pkt(60002, 8888+3000), true)
	require.True(t, ok)
	require.Equal(t, out.SequenceNumber+1, out2.SequenceNumber)
	require.Equal(t, out.Timestamp+3000, out2.Timestamp)
}

func TestVP8Keyframe(t *testing.T) {

	require.True(t, vp8Keyframe([]byte{0x10, 0x00, 0x00, 0x00}))

	require.False(t, vp8Keyframe([]byte{0x10, 0x01, 0x00, 0x00}))

	require.False(t, vp8Keyframe([]byte{0x00, 0x00, 0x00, 0x00}))
	require.False(t, vp8Keyframe(nil))
}

func TestH264Keyframe(t *testing.T) {
	require.True(t, h264Keyframe([]byte{0x67}), "SPS nal type 7")
	require.True(t, h264Keyframe([]byte{0x65}), "IDR nal type 5")
	require.False(t, h264Keyframe([]byte{0x61}), "non-IDR slice nal type 1")

	require.True(t, h264Keyframe([]byte{0x7c, 0x85}))

	require.False(t, h264Keyframe([]byte{0x7c, 0x05}))
	require.False(t, h264Keyframe(nil))
}

func TestRidBetter(t *testing.T) {
	require.True(t, ridBetter("h", ""))
	require.True(t, ridBetter("h", "m"))
	require.True(t, ridBetter("m", "l"))
	require.False(t, ridBetter("l", "h"))
	require.False(t, ridBetter("m", "h"))
}
