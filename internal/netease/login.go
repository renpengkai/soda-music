package netease

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"

	qrcode "github.com/skip2/go-qrcode"
)

// QRState is what the login QR code is doing right now.
type QRState int

const (
	// QRWaiting means the code is on screen and nobody has scanned it yet.
	QRWaiting QRState = iota
	// QRScanned means the code was scanned and the phone is asking to confirm.
	QRScanned
	// QRConfirmed means the phone confirmed and the session is live.
	QRConfirmed
	// QRExpired means the code timed out and a new one is needed.
	QRExpired
)

// QRLogin is one attempt at signing in with a QR code.
type QRLogin struct {
	// Key identifies the code on the server.
	Key string
	// Image is the PNG of the code, ready to draw.
	Image []byte
	// State is the last status the server reported.
	State QRState
	// Message is the server's own wording of the state.
	Message string
	// Cookie is the login cookie, set once the phone confirms.
	Cookie string
}

// QRKey asks for a fresh code and renders it. The code points at the service's
// own login page, so a phone camera opens the app rather than a URL.
func (c *Client) QRKey() (*QRLogin, error) {
	raw, err := c.call("/api/login/qrcode/unikey", map[string]any{"type": 1})
	if err != nil {
		return nil, err
	}
	var reply struct {
		Code   int    `json:"code"`
		Unikey string `json:"unikey"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, err
	}
	if reply.Code != 200 || reply.Unikey == "" {
		return nil, fmt.Errorf("netease: 无法获取登录二维码（%d）", reply.Code)
	}
	image, err := qrPNG("https://music.163.com/login?codekey=" + reply.Unikey)
	if err != nil {
		return nil, err
	}
	return &QRLogin{Key: reply.Unikey, Image: image, State: QRWaiting, Message: "请使用网易云音乐 App 扫码"}, nil
}

// QRCheck polls the state of a code. It returns an updated copy; the caller
// keeps polling until the state is QRConfirmed or QRExpired.
func (c *Client) QRCheck(login *QRLogin) (*QRLogin, error) {
	if login == nil || login.Key == "" {
		return nil, errors.New("netease: 没有待检查的二维码")
	}
	raw, err := c.call("/api/login/qrcode/client/login", map[string]any{"key": login.Key, "type": 1})
	if err != nil {
		return nil, err
	}
	var reply qrStatus
	if err := json.Unmarshal(raw, &reply); err != nil {
		return nil, err
	}
	next := *login
	next.Message = reply.Message
	switch reply.Code {
	case 800:
		next.State = QRExpired
		next.Message = "二维码已过期，请刷新"
	case 801:
		next.State = QRWaiting
		next.Message = "请使用网易云音乐 App 扫码"
	case 802:
		next.State = QRScanned
		next.Message = "已扫码，请在手机上确认登录"
	case 803:
		next.State = QRConfirmed
		next.Message = "登录成功"
		cookie := reply.Cookie
		if cookie == "" {
			cookie = c.Cookie()
		}
		next.Cookie = mergeCookie(c.Cookie(), cookie)
		c.SetCookie(next.Cookie)
	default:
		return nil, fmt.Errorf("netease: 未知的登录状态 %d", reply.Code)
	}
	return &next, nil
}

// qrPNG renders text as a QR code PNG. The code is drawn with a quiet zone so a
// phone camera can lock onto it against a light or dark panel.
func qrPNG(text string) ([]byte, error) {
	code, err := qrcode.New(text, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	code.DisableBorder = false
	bitmap := code.Bitmap()
	side := len(bitmap)
	if side == 0 {
		return nil, errors.New("netease: 空二维码")
	}
	scale := 8
	picture := image.NewRGBA(image.Rect(0, 0, side*scale, side*scale))
	for y := 0; y < side; y++ {
		for x := 0; x < side; x++ {
			shade := color.RGBA{R: 255, G: 255, B: 255, A: 255}
			if bitmap[y][x] {
				shade = color.RGBA{R: 0x0B, G: 0x1A, B: 0x17, A: 255}
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					picture.Set(x*scale+dx, y*scale+dy, shade)
				}
			}
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, picture); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
