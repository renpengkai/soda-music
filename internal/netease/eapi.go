package netease

import (
	"bytes"
	"crypto/aes"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// The eapi protocol is what the official desktop client speaks. A request is a
// JSON payload encrypted with a fixed AES key and signed with an MD5 digest of
// the payload itself, so the server can tell a genuine client from a tampered
// one. The scheme is not secret - the key ships inside the client - but it is
// what the endpoints expect, so it is reproduced here.
const (
	eapiKey  = "e82ckenh8dichen8"
	eapiHost = "https://interface3.music.163.com"
	eapiSep  = "-36cd479b6b5-"

	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	referer   = "https://music.163.com/"
)

// eapiParams turns a payload into the hex-encoded params field of an eapi
// request. path is the API path without the /eapi prefix, such as
// "/api/v6/playlist/detail".
func eapiParams(path string, payload map[string]any) (string, error) {
	text, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	message := "nobody" + path + "use" + string(text) + "md5forencrypt"
	sum := md5.Sum([]byte(message))
	digest := hex.EncodeToString(sum[:])
	plain := path + eapiSep + string(text) + eapiSep + digest
	encrypted, err := aesECBEncrypt([]byte(plain), []byte(eapiKey))
	if err != nil {
		return "", err
	}
	return strings.ToUpper(hex.EncodeToString(encrypted)), nil
}

// aesECBEncrypt encrypts data with AES in ECB mode and PKCS#7 padding, the shape
// the eapi protocol uses.
func aesECBEncrypt(data, key []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	size := block.BlockSize()
	data = pkcs7Pad(data, size)
	out := make([]byte, len(data))
	for offset := 0; offset < len(data); offset += size {
		block.Encrypt(out[offset:offset+size], data[offset:offset+size])
	}
	return out, nil
}

func pkcs7Pad(data []byte, size int) []byte {
	padding := size - len(data)%size
	return append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

// Client speaks to the NetEase Cloud Music eapi endpoints. It is safe to use
// from several goroutines at once; the cookie is the only mutable field and is
// read once per request.
type Client struct {
	http   *http.Client
	cookie string
}

// New makes a client. cookie may be empty for anonymous requests.
func New(cookie string) *Client {
	return &Client{
		http: &http.Client{
			Timeout: 20 * time.Second,
		},
		cookie: strings.TrimSpace(cookie),
	}
}

// SetCookie replaces the login cookie.
func (c *Client) SetCookie(cookie string) { c.cookie = strings.TrimSpace(cookie) }

// Cookie is the current login cookie, empty when signed out.
func (c *Client) Cookie() string { return c.cookie }

// headerCookie is the cookie actually sent: the login cookie with the client
// identity fields eapi requires, unless those are already present.
func (c *Client) headerCookie() string {
	out := c.cookie
	if !strings.Contains(out, "os=") {
		out = "os=pc; " + out
	}
	if !strings.Contains(out, "appver=") {
		out = "appver=8.9.70; " + out
	}
	return out
}

// call posts an eapi request and returns the raw JSON body. Any cookie the
// server sets is merged into the client, so a login is remembered.
func (c *Client) call(path string, payload map[string]any) (json.RawMessage, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	params, err := eapiParams(path, payload)
	if err != nil {
		return nil, err
	}
	body := "params=" + url.QueryEscape(params)
	request, err := http.NewRequest(http.MethodPost, eapiHost+"/eapi"+path, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.Header.Set("User-Agent", userAgent)
	request.Header.Set("Referer", referer)
	request.Header.Set("Cookie", c.headerCookie())

	response, err := c.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("netease: %s 返回 %d", path, response.StatusCode)
	}
	c.absorbCookies(response.Cookies())
	if len(raw) == 0 {
		return nil, errors.New("netease: 空响应")
	}
	return json.RawMessage(raw), nil
}

// absorbCookies merges the cookies a response set into the stored cookie, so a
// session established by a login request survives the next call.
func (c *Client) absorbCookies(cookies []*http.Cookie) {
	if len(cookies) == 0 {
		return
	}
	jar := parseCookie(c.cookie)
	for _, cookie := range cookies {
		if cookie == nil || cookie.Name == "" {
			continue
		}
		jar[cookie.Name] = cookie.Value
	}
	c.cookie = formatCookie(jar)
}
