package audio

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"testing"
	"time"
)

func TestHuyaSign(t *testing.T) {
	raw := "http://tx.flv.huya.com/src/1074-1074-4616-2161-10057-A-0-1-imgplus.flv?wsSecret=old&wsTime=6b067ffa&fm=RFdxOEJjSjNoNkRKdDZUWV8kMF8kMV8kMl8kMw%3D%3D&ctype=huya_live&fs=bgct"
	now := time.UnixMilli(1795587000000)
	out, err := huyaSign(raw, now)
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(out)
	q := u.Query()
	for _, k := range []string{"wsSecret", "wsTime", "seqid", "ctype", "ver", "fs", "fm", "t", "u"} {
		if q.Get(k) == "" {
			t.Fatalf("missing %s in %s", k, out)
		}
	}
	// recompute: prefix of base64("DWq8BcJ3h6DJt6TY_$0_$1_$2_$3") is DWq8BcJ3h6DJt6TY
	sum := func(s string) string { h := md5.Sum([]byte(s)); return hex.EncodeToString(h[:]) }
	want := sum(fmt.Sprintf("DWq8BcJ3h6DJt6TY_%s_1074-1074-4616-2161-10057-A-0-1-imgplus_%s_6b067ffa", q.Get("u"), sum(q.Get("seqid")+"|huya_live|100")))
	if q.Get("wsSecret") != want || q.Get("wsSecret") == "old" {
		t.Fatalf("wsSecret %s, want %s", q.Get("wsSecret"), want)
	}
	if _, err := huyaSign("http://x.huya.com/a.flv?fm=%%%", now); err == nil {
		t.Fatal("accepted a broken address")
	}
}
