package audio

// tsPTS reads the presentation time of audio PES packets out of an MPEG-TS byte
// stream fed in arbitrary pieces.
type tsPTS struct{ carry []byte }

// feed returns the first audio PTS (seconds) starting in b, if any.
func (t *tsPTS) feed(b []byte) (float64, bool) {
	buf := append(t.carry, b...)
	first, found := 0.0, false
	i := 0
	for ; i+188 <= len(buf); i += 188 {
		if buf[i] != 0x47 { // lost sync: find the next packet start
			for i < len(buf) && buf[i] != 0x47 {
				i++
			}
			i -= 188
			continue
		}
		pkt := buf[i : i+188]
		if pkt[1]&0x40 == 0 { // no payload unit starts here
			continue
		}
		p := 4
		if pkt[3]&0x20 != 0 { // adaptation field
			p += 1 + int(pkt[4])
		}
		if pkt[3]&0x10 == 0 || p+14 > 188 {
			continue
		}
		pes := pkt[p:]
		if pes[0] != 0 || pes[1] != 0 || pes[2] != 1 || pes[3] < 0xC0 || pes[3] > 0xDF || pes[7]&0x80 == 0 {
			continue // not an audio PES with a PTS
		}
		if pts, ok := ptsAt(pes[9:14]); ok && !found {
			first, found = pts, true
		}
	}
	t.carry = append(t.carry[:0], buf[min(i, len(buf)):]...)
	return first, found
}

func ptsAt(b []byte) (float64, bool) {
	if b[0]&1 == 0 || b[2]&1 == 0 || b[4]&1 == 0 { // marker bits
		return 0, false
	}
	v := uint64(b[0]>>1&7)<<30 | uint64(b[1])<<22 | uint64(b[2]>>1)<<15 | uint64(b[3])<<7 | uint64(b[4]>>1)
	return float64(v) / 90000, true
}
