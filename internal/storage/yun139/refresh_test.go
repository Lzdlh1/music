package yun139

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

// makeAuth 构造一个合成 Authorization：base64("pc:账号:随机|1|RCS|过期毫秒|E.体")
func makeAuth(account string, expire time.Time) string {
	token := "0mMrWUTd|1|RCS|" + itoa(expire.UnixMilli()) + "|E.abcdefgh.ijklmnop.qrstuvwx"
	return base64.StdEncoding.EncodeToString([]byte("pc:" + account + ":" + token))
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var b []byte
	for v > 0 {
		b = append([]byte{byte('0' + v%10)}, b...)
		v /= 10
	}
	return string(b)
}

func TestParseAuthorizationExpiry(t *testing.T) {
	want := time.UnixMilli(1789467603949)
	auth := makeAuth("18890088050", want)

	got, err := ParseAuthorizationExpiry(auth)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if !got.Equal(want) {
		t.Fatalf("过期时间不符: got=%s want=%s", got, want)
	}

	// 兼容带 Basic 前缀的写法
	if got2, err := ParseAuthorizationExpiry("Basic " + auth); err != nil || !got2.Equal(want) {
		t.Fatalf("Basic 前缀解析失败: %v %s", err, got2)
	}

	// 异常输入应报错而非 panic
	for _, bad := range []string{"", "not-base64!!", base64.StdEncoding.EncodeToString([]byte("pc:acct")),
		base64.StdEncoding.EncodeToString([]byte("pc:acct:only|two"))} {
		if _, err := ParseAuthorizationExpiry(bad); err == nil {
			t.Fatalf("异常输入 %q 应报错", bad)
		}
	}
}

func TestSplitAuthorization(t *testing.T) {
	auth := makeAuth("18890088050", time.Now())
	prefix, account, token, err := splitAuthorization(auth)
	if err != nil {
		t.Fatalf("拆分失败: %v", err)
	}
	if prefix != "pc" || account != "18890088050" {
		t.Fatalf("前缀/账号不符: %q %q", prefix, account)
	}
	if !strings.HasPrefix(token, "0mMrWUTd|1|RCS|") {
		t.Fatalf("token 不符: %q", token)
	}
}

func TestNeedRefresh(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		expire time.Time
		want   bool
	}{
		{"已过期", now.Add(-time.Hour), true},
		{"剩余 1 天", now.Add(24 * time.Hour), true},
		{"剩余 14 天", now.Add(14 * 24 * time.Hour), true},
		{"剩余 16 天", now.Add(16 * 24 * time.Hour), false},
		{"剩余 29 天", now.Add(29 * 24 * time.Hour), false},
	}
	for _, c := range cases {
		if got := NeedRefresh(c.expire, now); got != c.want {
			t.Errorf("%s: NeedRefresh=%v want=%v", c.name, got, c.want)
		}
	}
}