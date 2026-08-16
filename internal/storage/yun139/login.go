package yun139

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// 从 yun.139.com 网页端逆向得到的登录常量
const (
	// user-njs.yun.139.com 个人云统一认证 host
	loginBaseURL = "https://user-njs.yun.139.com"
	// 登录请求/响应加密封装密钥（AES-256-CBC），来自网页端 app.js
	loginAESKey = "UqEZkrjCKfa02pP6jntzFmkzOz86zHUC"
)

// 登录方式 type 值（对应网页端 loginStyle 数组 ["passSMS","passPwd","SIM","authLoginToken","passTmpTicker","QRCode","passSMSHK"]）
const (
	LoginTypeSMS      = 0 // 短信验证码
	LoginTypePassword = 1 // 账号密码
	LoginTypeQR       = 5 // 二维码扫码
)

// LoginClient 移动云盘（yun.139.com）网页端登录客户端
type LoginClient struct {
	client    *http.Client
	log       *zap.Logger
	visitorID string // 设备指纹，服务端按此关联二维码会话
	skeyOnce  sync.Once
	skeyErr   error
	secretKey string // mcloud-skey：RSA 加密的随机 AESKey
}

// NewLoginClient 创建登录客户端
func NewLoginClient(log *zap.Logger) *LoginClient {
	if log == nil {
		log = zap.NewNop()
	}
	return &LoginClient{
		client:    &http.Client{Timeout: 30 * time.Second},
		log:       log,
		visitorID: randomHex(32),
	}
}

// ensureSecretKey 获取 RSA 公钥并生成 mcloud-skey（懒加载，只执行一次）
func (lc *LoginClient) ensureSecretKey(ctx context.Context) {
	lc.skeyOnce.Do(func() {
		// getRsaPublicKey 请求体同样需要加密（走 UqEZ AES-256-CBC），响应亦加密
		plainJSON := []byte(`{"clientCode":"10701","type":"1"}`)
		enc, err := encryptPayload(plainJSON)
		if err != nil {
			lc.skeyErr = err
			return
		}
		sign := calSign(string(plainJSON))
		raw, err := lc.postRawInternal(ctx, "https://yun.139.com/orchestration/auth-rebuild/key/v1.0/getRsaPublicKey",
			enc, sign)
		if err != nil {
			lc.skeyErr = err
			return
		}
		// 解密响应
		respBytes := []byte(strings.TrimSpace(string(raw)))
		if !strings.HasPrefix(string(respBytes), "{") {
			pt, derr := decryptPayload(string(respBytes))
			if derr != nil {
				lc.skeyErr = fmt.Errorf("yun139: 解密公钥响应失败: %v, raw: %s", derr, string(raw))
				return
			}
			respBytes = pt
		}
		var out struct {
			Success bool `json:"success"`
			Data    struct {
				PublicKey string `json:"publicKey"`
			} `json:"data"`
		}
		if err := json.Unmarshal(respBytes, &out); err != nil || out.Data.PublicKey == "" {
			lc.skeyErr = fmt.Errorf("yun139: 获取 RSA 公钥失败: %s", string(respBytes))
			return
		}
		lc.secretKeyFrom(out.Data.PublicKey)
	})
}

// secretKeyFrom 用 RSA 公钥加密随机 AESKey 生成 mcloud-skey
func (lc *LoginClient) secretKeyFrom(publicKey string) {
	aesKey := randomString(16)
	skey, err := rsaEncryptPKCS1(aesKey, publicKey)
	if err != nil {
		lc.skeyErr = err
		return
	}
	lc.secretKey = skey
}

// rsaEncryptPKCS1 使用 RSA 公钥 PKCS#1 v1.5 加密（对应前端 jsencrypt）
func rsaEncryptPKCS1(data, publicKey string) (string, error) {
	var pub *rsa.PublicKey
	der := []byte(publicKey)
	// 兼容 PEM 与裸 base64 两种格式
	if block, _ := pem.Decode(der); block != nil {
		der = block.Bytes
	} else if b, err := base64.StdEncoding.DecodeString(publicKey); err == nil {
		der = b
	}
	if k, err := x509.ParsePKIXPublicKey(der); err == nil {
		if rk, ok := k.(*rsa.PublicKey); ok {
			pub = rk
		}
	}
	if pub == nil {
		if k, err := x509.ParsePKCS1PublicKey(der); err == nil {
			pub = k
		}
	}
	if pub == nil {
		return "", fmt.Errorf("yun139: 无法解析 RSA 公钥")
	}
	ct, err := rsa.EncryptPKCS1v15(crand.Reader, pub, []byte(data))
	if err != nil {
		return "", fmt.Errorf("yun139: RSA 加密失败: %v", err)
	}
	return base64.StdEncoding.EncodeToString(ct), nil
}

// randomHex 生成 n 位随机 hex 字符串
func randomHex(n int) string {
	const hexChars = "0123456789abcdef"
	b := make([]byte, n)
	seed := uint64(time.Now().UnixNano())
	for i := range b {
		seed = seed*6364136223846793005 + 1442695040888963407
		b[i] = hexChars[(seed>>33)%16]
	}
	return string(b)
}

// QRLoginInfo 二维码登录信息
type QRLoginInfo struct {
	SID     string `json:"sid"`     // 会话 ID（二维码中的 sID）
	Content string `json:"content"` // 二维码内容 URL
}

// LoginResult 登录成功结果
type LoginResult struct {
	Authorization string `json:"authorization"` // 用于 hcy 接口的 Authorization（不含 Basic 前缀）
	Account       string `json:"account"`       // 完整账号（手机号）
	AuthToken     string `json:"auth_token"`
	UserDomainID  string `json:"user_domain_id"` // 用户域 ID
	PersonalHost  string `json:"personal_host"`  // 个人云动态 host（登录响应 routerInfo 中提取）
}

// LoginError 登录错误（含平台错误码）
type LoginError struct {
	Code    string
	Message string
}

func (e *LoginError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("yun139: %s (%s)", e.Message, e.Code)
	}
	return fmt.Sprintf("yun139 login failed, code: %s", e.Code)
}

// QRStatusText 二维码轮询状态码含义
func QRStatusText(code string) string {
	switch code {
	case "200059541":
		return "等待扫码，请使用中国移动云盘APP或微信扫码"
	case "200059548":
		return "已扫码，请在手机上确认"
	case "200059549":
		return "已取消登录"
	case "200059542":
		return "二维码已失效，请刷新"
	case "200059543", "200059545", "200059546", "200059547":
		return "扫码失败，请重试"
	case "0", "0000", "200":
		return "登录成功"
	default:
		return ""
	}
}

// ---------- 加密 ----------

func sha1Hex(s string) string {
	h := sha1.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// pkcs7Pad/Unpad
func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	pad := int(data[len(data)-1])
	if pad > len(data) || pad == 0 || pad > aes.BlockSize {
		return data, nil
	}
	return data[:len(data)-pad], nil
}

// encryptPayload 加密请求体：Base64(随机16字节IV + AES-256-CBC密文)
func encryptPayload(plaintext []byte) (string, error) {
	key := []byte(loginAESKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	iv := make([]byte, aes.BlockSize)
	if _, err := io.ReadFull(randReader(), iv); err != nil {
		return "", err
	}
	padded := pkcs7Pad(plaintext, aes.BlockSize)
	ct := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ct, padded)
	return base64.StdEncoding.EncodeToString(append(iv, ct...)), nil
}

// decryptPayload 解密响应体：Base64(IV + 密文)
func decryptPayload(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
	if err != nil {
		return nil, err
	}
	if len(raw) < aes.BlockSize {
		return nil, fmt.Errorf("payload too short")
	}
	key := []byte(loginAESKey)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(raw)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("payload length not multiple of block size")
	}
	iv, ct := raw[:aes.BlockSize], raw[aes.BlockSize:]
	pt := make([]byte, len(ct))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(pt, ct)
	return pkcs7Unpad(pt)
}

// randReader 便于测试替换
var randReader = func() io.Reader { return &randSource{} }

type randSource struct{}

func (r *randSource) Read(p []byte) (int, error) {
	// 使用 time+纳秒伪随机填充（无密码学强度要求）
	n := 0
	seed := time.Now().UnixNano()
	for n < len(p) {
		seed = seed*6364136223846793005 + 1442695040888963407
		p[n] = byte(seed >> 33)
		n++
	}
	return n, nil
}

// ---------- HTTP ----------

func (lc *LoginClient) postRaw(ctx context.Context, path, body, sign string) ([]byte, error) {
	// 确保 mcloud-skey 可用（懒加载）
	lc.ensureSecretKey(ctx)
	if lc.skeyErr != nil {
		return nil, lc.skeyErr
	}
	raw, err := lc.postRawInternal(ctx, path, body, sign)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// postRawInternal 执行实际 POST（不触发 ensureSecretKey，避免递归）
func (lc *LoginClient) postRawInternal(ctx context.Context, path, body, sign string) ([]byte, error) {

	fullURL := path
	if !strings.HasPrefix(path, "http") {
		fullURL = loginBaseURL + path
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, fullURL, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/142.0.0.0 Safari/537.36")
	req.Header.Set("caller", "web")
	req.Header.Set("hcy-cool-flag", "1")
	req.Header.Set("CMS-DEVICE", "default")
	req.Header.Set("x-yun-api-version", "v1")
	req.Header.Set("x-yun-svc-type", "1")
	req.Header.Set("x-SvcType", "1")
	req.Header.Set("x-yun-module-type", "100")
	req.Header.Set("x-yun-app-channel", "10000034")
	req.Header.Set("x-yun-channel-source", "10000034")
	req.Header.Set("x-m4c-caller", "PC")
	req.Header.Set("x-m4c-src", "10002")
	req.Header.Set("x-inner-ntwk", "2")
	req.Header.Set("mcloud-route", "001")
	req.Header.Set("mcloud-channel", "1000101")
	req.Header.Set("mcloud-client", "10701")
	req.Header.Set("mcloud-version", "7.17.9")
	req.Header.Set("x-huawei-channelSrc", "10000034")
	// 设备指纹头：服务端按 visitorId 关联二维码登录会话
	if lc.visitorID != "" {
		req.Header.Set("X-Deviceinfo", "||9|7.17.9|chrome|142.0.7444.235|"+lc.visitorID+"||windows 10||zh-CN|||")
		req.Header.Set("x-yun-client-info", "||9|7.17.9|chrome|142.0.7444.235|"+lc.visitorID+"||windows 10||zh-CN|||undefined||")
	}
	// mcloud-sign 基于明文请求体计算（字符排序 + md5），网页端与个人云接口通用
	if sign == "" {
		sign = calSign(body)
	}
	req.Header.Set("mcloud-sign", sign)
	if lc.secretKey != "" {
		req.Header.Set("mcloud-skey", lc.secretKey)
	}
	req.Header.Set("INNER-HCY-ROUTER-HTTPS", "1")
	req.Header.Set("Origin", "https://yun.139.com")
	req.Header.Set("Referer", "https://yun.139.com/w/")

	resp, err := lc.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}
	lc.log.Debug("yun139 login api",
		zap.String("path", path),
		zap.Int("status", resp.StatusCode),
		zap.ByteString("body", raw))
	return raw, nil
}

// thirdLoginResp thirdlogin 解密后的响应
type thirdLoginResp struct {
	Success bool            `json:"success"`
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// aesECBDecrypt AES-128-ECB 解密（登录响应 data 第二层解密）
func aesECBDecrypt(ciphertext []byte, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("aes ecb: ciphertext is not a multiple of block size")
	}
	pt := make([]byte, len(ciphertext))
	for i := 0; i < len(ciphertext); i += aes.BlockSize {
		block.Decrypt(pt[i:i+aes.BlockSize], ciphertext[i:i+aes.BlockSize])
	}
	return pkcs7Unpad(pt)
}

// postEncrypted 加密请求并解密响应（两层：body AES-256-CBC，data 字段 AES-128-ECB）
func (lc *LoginClient) postEncrypted(ctx context.Context, path string, plain interface{}) (*thirdLoginResp, error) {
	plainJSON, err := json.Marshal(plain)
	if err != nil {
		return nil, err
	}
	body, err := encryptPayload(plainJSON)
	if err != nil {
		return nil, err
	}
	sign := calSign(string(plainJSON))
	raw, err := lc.postRaw(ctx, path, body, sign)
	if err != nil {
		return nil, err
	}
	// 响应可能为加密密文，也可能直接是明文 JSON（兼容）
	trimmed := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(trimmed, "{") {
		pt, err := decryptPayload(trimmed)
		if err != nil {
			return nil, fmt.Errorf("yun139 login: 响应解密失败: %v", err)
		}
		trimmed = string(pt)
	}
	lc.log.Debug("yun139 login response decrypted", zap.String("plain", trimmed))
	var out thirdLoginResp
	if err := json.Unmarshal([]byte(trimmed), &out); err != nil {
		return nil, fmt.Errorf("yun139 login: 解密结果解析失败: %v, raw: %s", err, trimmed)
	}
	// 第二层：data 为 hex 密文时用固定 key AES-128-ECB 解密
	if len(out.Data) > 0 && string(out.Data) != "null" {
		dataStr := strings.Trim(string(out.Data), `"`)
		if !strings.HasPrefix(dataStr, "{") {
			ct, herr := hex.DecodeString(dataStr)
			if herr == nil {
				pt2, derr := aesECBDecrypt(ct, []byte("qPqDw263XgFgL3u8"))
				if derr == nil {
					out.Data = pt2
				}
			}
		}
	}
	return &out, nil
}

// ---------- 登录 ----------

// buildAuth 根据账号与 authToken 构造 hcy 接口使用的 Authorization（不含 Basic 前缀）
func buildAuth(account, authToken string) string {
	return base64.StdEncoding.EncodeToString([]byte("pc:" + account + ":" + authToken))
}

// thirdLogin 统一登录请求。
// loginType: 0=短信验证码, 1=账号密码, 5=二维码扫码
// pintype 映射（网页端 loginDataPrecode）：短信=5(香港8位号23), 密码=9, 二维码=21
func (lc *LoginClient) thirdLogin(ctx context.Context, account, dycPwd string, loginType int) (*LoginResult, error) {
	pintype := 5
	switch loginType {
	case LoginTypeSMS:
		if len(account) == 8 {
			pintype = 23
		}
	case LoginTypePassword:
		pintype = 9
	case LoginTypeQR:
		pintype = 21
	}
	// 网页端 c 函数结构：random 仅密码登录为 16 位随机大写
	random := ""
	if loginType == LoginTypePassword {
		random = strings.ToUpper(randomString(16))
	}
	out, err := lc.postEncrypted(ctx, "/user/thirdlogin", map[string]interface{}{
		"msisdn":     account,
		"random":     random,
		"dycpwd":     dycPwd,
		"cpid":       292,
		"clienttype": 670,
		"version":    "mCloud_4.3.0_536",
		"pintype":    pintype,
		"secinfo":    strings.ToUpper(sha1Hex("fetion.com.cn:" + dycPwd)),
		"loginMode":  "0",
		"extInfo":    map[string]interface{}{},
	})
	if err != nil {
		return nil, err
	}

	// 解析 data
	var data struct {
		Account        string          `json:"account"`
		Token          string          `json:"token"`
		AuthToken      string          `json:"authToken"`
		EncryptAccount string          `json:"encryptAccount"`
		UserDomainID   string          `json:"userDomainId"`
		RouterInfo     json.RawMessage `json:"routerInfo"`
	}
	if len(out.Data) > 0 && string(out.Data) != "null" {
		_ = json.Unmarshal(out.Data, &data)
	}
	authToken := data.AuthToken
	if authToken == "" {
		authToken = data.Token
	}
	if authToken == "" {
		msg := out.Message
		code := out.Code
		if code == "0000" {
			code = "0"
		}
		return nil, &LoginError{Code: code, Message: msg}
	}

	// 解析账号：优先 encryptAccount（base64 编码），其次 account 字段，最后回退请求账号
	fullAccount := data.Account
	if fullAccount == "" && data.EncryptAccount != "" {
		if b, err := base64.StdEncoding.DecodeString(data.EncryptAccount); err == nil {
			fullAccount = string(b)
		}
	}
	if fullAccount == "" {
		fullAccount = account
	}
	return &LoginResult{
		Authorization: buildAuth(fullAccount, authToken),
		Account:       fullAccount,
		AuthToken:     authToken,
		UserDomainID:  data.UserDomainID,
		PersonalHost:  extractPersonalHost(data.RouterInfo),
	}, nil
}

// extractPersonalHost 从登录响应的 routerInfo（路由列表）提取个人云 host
func extractPersonalHost(routerInfo json.RawMessage) string {
	if len(routerInfo) == 0 || string(routerInfo) == "null" {
		return ""
	}
	var list []struct {
		ModName string `json:"modName"`
		HTTPS   string `json:"httpsUrl"`
	}
	if err := json.Unmarshal(routerInfo, &list); err != nil {
		return ""
	}
	for _, r := range list {
		if r.ModName == "personal" && r.HTTPS != "" {
			return strings.TrimSuffix(r.HTTPS, "/")
		}
	}
	return ""
}

// GetSmsCode 发送短信验证码，返回 random（供登录时使用）
func (lc *LoginClient) GetSmsCode(ctx context.Context, account string) (string, error) {
	body := map[string]interface{}{
		"phoneNumber": account,
		"random":      strconv.FormatInt(time.Now().UnixMilli(), 10),
		"nationCode":  "+86",
		"clientType":  670,
	}
	plainJSON, _ := json.Marshal(body)
	raw, err := lc.postRaw(ctx, "/user/sms/getSmsCode", string(plainJSON), calSign(string(plainJSON)))
	if err != nil {
		return "", err
	}
	var out struct {
		Success bool            `json:"success"`
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		// 尝试解密响应
		pt, derr := decryptPayload(strings.TrimSpace(string(raw)))
		if derr != nil {
			return "", fmt.Errorf("yun139: 发送验证码响应解析失败: %v, raw: %s", err, string(raw))
		}
		if err := json.Unmarshal(pt, &out); err != nil {
			return "", fmt.Errorf("yun139: 发送验证码响应解析失败: %v", err)
		}
	}
	code := out.Code
	if code == "" && out.Success {
		code = "0"
	}
	if code != "" && code != "0" && code != "0000" && !out.Success {
		return "", &LoginError{Code: code, Message: out.Message}
	}
	var data struct {
		Random string `json:"random"`
	}
	if len(out.Data) > 0 {
		_ = json.Unmarshal(out.Data, &data)
	}
	if data.Random == "" {
		data.Random = body["random"].(string)
	}
	return data.Random, nil
}

// SmsLogin 短信验证码登录
func (lc *LoginClient) SmsLogin(ctx context.Context, account, code string) (*LoginResult, error) {
	return lc.thirdLogin(ctx, account, code, LoginTypeSMS)
}

// PasswordLogin 账号密码登录
func (lc *LoginClient) PasswordLogin(ctx context.Context, account, password string) (*LoginResult, error) {
	return lc.thirdLogin(ctx, account, password, LoginTypePassword)
}

// StartQRLogin 生成扫码登录信息（二维码内容 + sID），二维码 dID 绑定设备指纹
func (lc *LoginClient) StartQRLogin() (*QRLoginInfo, error) {
	sid := randomString(16)
	content := fmt.Sprintf("https://yun.139.com/w/#/qrcLogin?sID=%s&dID=%s&cType=9", sid, lc.visitorID)
	return &QRLoginInfo{SID: sid, Content: content}, nil
}

// PollQRLogin 轮询扫码登录状态。
// 返回 nil 表示登录成功；返回 *LoginError（Code 为平台状态码）表示等待中/已取消/已失效。
func (lc *LoginClient) PollQRLogin(ctx context.Context, sid string) (*LoginResult, error) {
	return lc.thirdLogin(ctx, "", sid, LoginTypeQR)
}
