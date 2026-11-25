package audio

import (
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/rand/v2"
	"net/url"
	"path"
	"strings"
	"time"
)

// huyaSign re-signs a Huya FLV address. The anti-hotlink parameters yt-dlp builds
// lack seqid/u/t/ver, and Huya's CDN then sends just its cached first second and
// stops; computing the signature the way the web player does keeps the stream
// flowing (checked: 26 s of audio in a 30 s window, against 1 s unsigned).
//
// wsSecret = md5(prefix_u_stream_md5(seqid|ctype|t)_wsTime), where prefix is the
// first part of the base64 "fm" parameter, u a viewer id with its low 32 bits
// rotated left by 8, and seqid = viewer id + now in ms.
func huyaSign(raw string, now time.Time) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	q := u.Query()
	fm, err := base64.StdEncoding.DecodeString(q.Get("fm"))
	if err != nil || q.Get("wsTime") == "" {
		return "", fmt.Errorf("huya: unexpected stream address")
	}
	prefix, _, _ := strings.Cut(string(fm), "_")
	stream := strings.TrimSuffix(path.Base(u.Path), path.Ext(u.Path))
	ctype := q.Get("ctype")
	if ctype == "" {
		ctype = "huya_live"
	}
	viewer := uint64(1_400_000_000_000 + rand.IntN(10_000_000)) // an anonymous web viewer
	low := viewer & 0xFFFFFFFF
	conv := viewer>>32<<32 | (low<<8|low>>24)&0xFFFFFFFF
	seqid := viewer + uint64(now.UnixMilli())
	const t = "100" // web
	sum := func(s string) string { h := md5.Sum([]byte(s)); return hex.EncodeToString(h[:]) }
	secret := sum(fmt.Sprintf("%s_%d_%s_%s_%s", prefix, conv, stream, sum(fmt.Sprintf("%d|%s|%s", seqid, ctype, t)), q.Get("wsTime")))
	nq := url.Values{}
	for k, v := range map[string]string{"wsSecret": secret, "wsTime": q.Get("wsTime"), "seqid": fmt.Sprint(seqid),
		"ctype": ctype, "ver": "1", "fs": q.Get("fs"), "fm": q.Get("fm"), "t": t, "u": fmt.Sprint(conv)} {
		nq.Set(k, v)
	}
	u.RawQuery = nq.Encode()
	return u.String(), nil
}
