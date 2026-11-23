package wechat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// DefaultAPIBase 微信小程序接口默认地址；测试或私有代理可通过 WECHAT_API_BASE 覆盖。
const DefaultAPIBase = "https://api.weixin.qq.com"

type Client struct {
	appID      string
	appSecret  string
	base       string
	httpClient *http.Client

	mu          sync.Mutex
	accessToken string
	expiresAt   time.Time
}

func NewClient(appID, appSecret, base string) *Client {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		base = DefaultAPIBase
	}
	return &Client{
		appID:      appID,
		appSecret:  appSecret,
		base:       base,
		httpClient: &http.Client{Timeout: 6 * time.Second},
	}
}

func (c *Client) Configured() bool {
	return c.appID != "" && c.appSecret != ""
}

type sessionResponse struct {
	OpenID     string `json:"openid"`
	UnionID    string `json:"unionid"`
	SessionKey string `json:"session_key"`
	ErrCode    int    `json:"errcode"`
	ErrMsg     string `json:"errmsg"`
}

func (c *Client) Code2Session(ctx context.Context, code string) (openID, unionID string, err error) {
	if !c.Configured() {
		return "", "", errors.New("wechat credentials not configured")
	}
	q := url.Values{}
	q.Set("appid", c.appID)
	q.Set("secret", c.appSecret)
	q.Set("js_code", code)
	q.Set("grant_type", "authorization_code")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+"/sns/jscode2session?"+q.Encode(), nil)
	if err != nil {
		return "", "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("wechat code2session: %w", err)
	}
	defer resp.Body.Close()

	var out sessionResponse
	if err := decodeJSON(resp.Body, &out); err != nil {
		return "", "", err
	}
	if out.ErrCode != 0 {
		return "", "", fmt.Errorf("wechat code2session: %d %s", out.ErrCode, out.ErrMsg)
	}
	if out.OpenID == "" {
		return "", "", errors.New("wechat code2session: empty openid")
	}
	return out.OpenID, out.UnionID, nil
}

type phoneResponse struct {
	ErrCode   int    `json:"errcode"`
	ErrMsg    string `json:"errmsg"`
	PhoneInfo struct {
		PhoneNumber     string `json:"phoneNumber"`
		PurePhoneNumber string `json:"purePhoneNumber"`
		CountryCode     string `json:"countryCode"`
	} `json:"phone_info"`
}

func (c *Client) GetPhoneNumber(ctx context.Context, code string) (string, error) {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return "", err
	}
	body, _ := json.Marshal(map[string]string{"code": code})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/wxa/business/getuserphonenumber?access_token="+url.QueryEscape(token),
		bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("wechat get phone: %w", err)
	}
	defer resp.Body.Close()

	var out phoneResponse
	if err := decodeJSON(resp.Body, &out); err != nil {
		return "", err
	}
	if out.ErrCode != 0 {
		return "", fmt.Errorf("wechat get phone: %d %s", out.ErrCode, out.ErrMsg)
	}
	if out.PhoneInfo.PurePhoneNumber != "" {
		return out.PhoneInfo.PurePhoneNumber, nil
	}
	if out.PhoneInfo.PhoneNumber != "" {
		return out.PhoneInfo.PhoneNumber, nil
	}
	return "", errors.New("wechat get phone: empty phone")
}

func (c *Client) SendSubscribeMessage(ctx context.Context, openID, templateID, page string, data map[string]string) error {
	token, err := c.getAccessToken(ctx)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"touser":      openID,
		"template_id": templateID,
		"page":        page,
		"data":        map[string]any{},
	}
	fields := payload["data"].(map[string]any)
	for key, value := range data {
		fields[key] = map[string]string{"value": value}
	}
	body, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.base+"/cgi-bin/message/subscribe/send?access_token="+url.QueryEscape(token),
		bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("wechat subscribe send: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return err
	}
	if out.ErrCode != 0 {
		return fmt.Errorf("wechat subscribe send: %d %s", out.ErrCode, out.ErrMsg)
	}
	return nil
}

func (c *Client) getAccessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.accessToken != "" && time.Now().Before(c.expiresAt.Add(-2*time.Minute)) {
		return c.accessToken, nil
	}
	q := url.Values{}
	q.Set("grant_type", "client_credential")
	q.Set("appid", c.appID)
	q.Set("secret", c.appSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+"/cgi-bin/token?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("wechat access token: %w", err)
	}
	defer resp.Body.Close()
	var out struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		ErrCode     int    `json:"errcode"`
		ErrMsg      string `json:"errmsg"`
	}
	if err := decodeJSON(resp.Body, &out); err != nil {
		return "", err
	}
	if out.ErrCode != 0 {
		return "", fmt.Errorf("wechat access token: %d %s", out.ErrCode, out.ErrMsg)
	}
	if out.AccessToken == "" {
		return "", errors.New("wechat access token: empty")
	}
	c.accessToken = out.AccessToken
	c.expiresAt = time.Now().Add(time.Duration(out.ExpiresIn) * time.Second)
	return c.accessToken, nil
}

func decodeJSON(r io.Reader, v any) error {
	body, err := io.ReadAll(io.LimitReader(r, 1<<20))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("decode wechat response: %w", err)
	}
	return nil
}
