// 139（中国移动云盘）凭据自动续期。
//
// Authorization 形如 base64("pc:手机号:token")，其中 token 自身又是
// "随机串|1|RCS|过期时间毫秒|E.加密体" 的分段结构，第 4 段即过期时间。
// 续期接口、15 天提前量阈值均与 OpenList 的 drivers/139 实现一致。
package yun139

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

const (
	// 续期接口：以当前 token 换取新 token，无需账号密码
	authTokenRefreshURL = "https://aas.caiyun.feixin.10086.cn:443/tellin/authTokenRefresh.do"
	// 剩余有效期低于该阈值时提前续期
	authRefreshAhead = 15 * 24 * time.Hour
	// 续期请求中的客户端类型标识
	authRefreshClientType = "656"
)

// splitAuthorization 把 Authorization 拆成 (前缀, 账号, token)；
// 兼容带 "Basic " 前缀的写法。
func splitAuthorization(auth string) (prefix, account, token string, err error) {
	raw := strings.TrimSpace(auth)
	if i := strings.IndexByte(raw, ' '); i >= 0 {
		raw = strings.TrimSpace(raw[i+1:])
	}
	decoded, derr := base64.StdEncoding.DecodeString(raw)
	if derr != nil {
		return "", "", "", fmt.Errorf("yun139: authorization 不是合法 base64: %w", derr)
	}
	splits := strings.Split(string(decoded), ":")
	if len(splits) < 3 {
		return "", "", "", errors.New("yun139: authorization 结构异常（冒号分段不足 3 段）")
	}
	return splits[0], splits[1], splits[2], nil
}

// ParseAuthorizationExpiry 解析 Authorization 内嵌的过期时间
func ParseAuthorizationExpiry(auth string) (time.Time, error) {
	_, _, token, err := splitAuthorization(auth)
	if err != nil {
		return time.Time{}, err
	}
	strs := strings.Split(token, "|")
	if len(strs) < 4 {
		return time.Time{}, errors.New("yun139: authorization 缺少过期时间字段")
	}
	ms, err := strconv.ParseInt(strs[3], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("yun139: 过期时间解析失败: %w", err)
	}
	return time.UnixMilli(ms), nil
}

type authRefreshResp struct {
	XMLName xml.Name `xml:"root"`
	Return  string   `xml:"return"`
	Token   string   `xml:"token"`
	Desc    string   `xml:"desc"`
}

// RefreshAuthorization 用当前 Authorization 换取新的 Authorization 与过期时间。
// token 已过期时服务端返回 return=4006，此时只能重新登录。
func RefreshAuthorization(ctx context.Context, auth string, log *zap.Logger) (string, time.Time, error) {
	if log == nil {
		log = zap.NewNop()
	}
	prefix, account, token, err := splitAuthorization(auth)
	if err != nil {
		return "", time.Time{}, err
	}

	body := fmt.Sprintf("<root><token>%s</token><account>%s</account><clienttype>%s</clienttype></root>",
		token, account, authRefreshClientType)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, authTokenRefreshURL, strings.NewReader(body))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/xml")
	req.Header.Set("Accept", "application/xml, text/xml, */*")
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("yun139: 续期请求失败: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("yun139: 续期响应读取失败: %w", err)
	}

	var out authRefreshResp
	if err := xml.Unmarshal(raw, &out); err != nil {
		return "", time.Time{}, fmt.Errorf("yun139: 续期响应解析失败: %w, body: %s", err, firstBytes(raw, 200))
	}
	if out.Return != "0" || out.Token == "" {
		// 4006 = token 不存在或已过期，需重新登录
		if out.Return == "4006" {
			return "", time.Time{}, fmt.Errorf("yun139: Token 已过期，无法自动续期，请重新登录该云盘")
		}
		return "", time.Time{}, fmt.Errorf("yun139: 续期被拒绝 (return=%s): %s", out.Return, out.Desc)
	}

	newAuth := base64.StdEncoding.EncodeToString([]byte(prefix + ":" + account + ":" + out.Token))
	expire, err := ParseAuthorizationExpiry(newAuth)
	if err != nil {
		log.Warn("yun139: 续期成功但无法解析新过期时间", zap.Error(err))
	}
	return newAuth, expire, nil
}

// NeedRefresh 判断是否该续期：已过期或剩余不足阈值
func NeedRefresh(expire time.Time, now time.Time) bool {
	return expire.Sub(now) < authRefreshAhead
}