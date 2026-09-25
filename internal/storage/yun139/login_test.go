package yun139

import (
	"crypto/aes"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// aesECBEncryptForTest 仅测试用：按生产代码相同的固定 key 做 AES-128-ECB 加密
func aesECBEncryptForTest(plain []byte) (string, error) {
	block, err := aes.NewCipher([]byte("qPqDw263XgFgL3u8"))
	if err != nil {
		return "", err
	}
	pad := aes.BlockSize - len(plain)%aes.BlockSize
	padded := append(append([]byte{}, plain...), bytesRepeat(byte(pad), pad)...)
	out := make([]byte, len(padded))
	for i := 0; i < len(padded); i += aes.BlockSize {
		block.Encrypt(out[i:i+aes.BlockSize], padded[i:i+aes.BlockSize])
	}
	return hex.EncodeToString(out), nil
}

func bytesRepeat(b byte, n int) []byte {
	s := make([]byte, n)
	for i := range s {
		s[i] = b
	}
	return s
}

// TestDecryptRespPayloadQuoted 回归测试：服务端返回 "base64密文"（带 JSON 引号）时，
// 必须能正确还原。此前 GetSmsCode/postEncrypted 直接把带引号的原文丢给
// decryptPayload，导致报错 "cannot unmarshal string into Go value of type struct".
func TestDecryptRespPayloadQuoted(t *testing.T) {
	want := `{"success":true,"message":"ok","data":{"random":"abc123"}}`
	enc, err := encryptPayload([]byte(want))
	if err != nil {
		t.Fatalf("构造密文失败: %v", err)
	}

	quoted, _ := json.Marshal(enc) // 等价于服务端返回 "base64密文"

	cases := []struct {
		name  string
		input []byte
	}{
		{"带 JSON 引号", quoted},
		{"裸密文", []byte(enc)},
		{"带引号且含首尾空白", []byte("  " + string(quoted) + "\n")},
		{"明文 JSON 应原样返回", []byte(want)},
	}

	for _, c := range cases {
		got, err := decryptRespPayload(c.input)
		if err != nil {
			t.Errorf("%s: 解密失败: %v", c.name, err)
			continue
		}
		if strings.TrimSpace(string(got)) != want {
			t.Errorf("%s: 结果不符\n got=%s\nwant=%s", c.name, got, want)
		}
	}
}

// TestDecryptRespPayloadInnerData 内层 data 为 hex 密文时应被 AES-128-ECB 解开
func TestDecryptRespPayloadInnerData(t *testing.T) {
	inner := `{"account":"18890088050","userDomainId":"1081467838811497103"}`
	ct, err := aesECBEncryptForTest([]byte(inner))
	if err != nil {
		t.Fatalf("构造内层密文失败: %v", err)
	}
	outer := `{"success":true,"data":"` + ct + `"}`
	enc, err := encryptPayload([]byte(outer))
	if err != nil {
		t.Fatalf("构造外层密文失败: %v", err)
	}
	quoted, _ := json.Marshal(enc)

	got, err := decryptRespPayload(quoted)
	if err != nil {
		t.Fatalf("解密失败: %v", err)
	}
	var out struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(got, &out); err != nil {
		t.Fatalf("解析失败: %v, got=%s", err, got)
	}
	if !strings.Contains(string(out.Data), "18890088050") {
		t.Fatalf("内层 data 未解密: %s", out.Data)
	}
}