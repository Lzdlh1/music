package source

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestMiguDecodeBodyRoundTrip 验证 H5 响应还原算法（与站点前端 encode 互为逆运算）
func TestMiguDecodeBodyRoundTrip(t *testing.T) {
	plain := []byte(`{"code":"000000","info":"操作成功","data":{"url":"https://example.com/a.mp3"}}`)
	key := []byte(miguCoderKey)

	seed := byte(0x3C)
	enc := make([]byte, 0, len(plain)+4)
	enc = append(enc, miguCoderHeader1, miguCoderHeader2, miguCoderVersion, seed)
	for i, b := range plain {
		enc = append(enc, b+key[i%len(key)]-seed)
	}

	if got := miguDecodeBody(enc); string(got) != string(plain) {
		t.Fatalf("decode mismatch:\n got=%s\nwant=%s", got, plain)
	}

	// 十六进制密文形态（请求头不完整时服务端返回）
	hexEnc := make([]byte, 0, len(enc)*2)
	const digits = "0123456789ABCDEF"
	for _, b := range enc {
		hexEnc = append(hexEnc, digits[b>>4], digits[b&0x0F])
	}
	if got := miguDecodeBody(hexEnc); string(got) != string(plain) {
		t.Fatalf("hex decode mismatch:\n got=%s\nwant=%s", got, plain)
	}

	// 非本格式的响应应原样返回
	raw := []byte("plain body")
	if got := miguDecodeBody(raw); string(got) != string(raw) {
		t.Fatalf("passthrough failed: %s", got)
	}
}

// TestMiguH5GetDownloadURL 真实调用咪咕 H5 播放接口，验证能拿到完整音频直链
func TestMiguH5GetDownloadURL(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}
	src := NewMiguSource(MiguConfig{Timeout: 20}, zap.NewNop())

	// 《晴天》周杰伦：白金会员曲目，PC 接口会拒绝，H5 接口应能下发直链
	id := miguTrackID("migu", "60054701923", "600902000006889366", "", QualityAny, "", "")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	dl, err := src.GetDownloadURL(ctx, id, Quality128)
	if err != nil {
		t.Fatalf("GetDownloadURL failed: %v", err)
	}
	t.Logf("url=%s", dl.URL)
	t.Logf("quality=%s format=%s size=%d", dl.Quality, dl.Format, dl.FileSize)
	if dl.URL == "" {
		t.Fatal("empty url")
	}
	if dl.FileSize < 1<<20 {
		t.Fatalf("size too small, likely an audition clip: %d", dl.FileSize)
	}
}