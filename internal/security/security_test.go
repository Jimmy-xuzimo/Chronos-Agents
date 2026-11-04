package security

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPasswordAndOrigin(t *testing.T) {
	h, e := Password("fictional-password-123")
	if e != nil || !VerifyPassword(h, "fictional-password-123") || VerifyPassword(h, "different-password") {
		t.Fatal("password verification")
	}
	for _, s := range []string{"http://remote.example.com", "https://user:pass@example.com", "https://example.com/path", "https://example.com?token=x"} {
		if _, e = Origin(s, true); e == nil {
			t.Fatal("unsafe origin accepted")
		}
	}
	if _, e = Origin("http://127.0.0.1:17870", true); e != nil {
		t.Fatal(e)
	}
	if _, e = Origin("https://hub.example.com", false); e != nil {
		t.Fatal(e)
	}
}
func TestDecodeRejectsTrailingAndEncoded(t *testing.T) {
	for _, body := range []string{`{"a":1} garbage`, `{"a":1} {"a":2}`, strings.Repeat("a", 300000)} {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		var out struct {
			A int `json:"a"`
		}
		if Decode(httptest.NewRecorder(), r, &out) == nil {
			t.Fatal("malformed payload accepted")
		}
	}
}
func TestLimiterBounded(t *testing.T) {
	var b Bucket
	for i := 0; i < 5; i++ {
		if !b.Allow("device", 5, time.Minute) {
			t.Fatal("premature limit")
		}
	}
	if b.Allow("device", 5, time.Minute) {
		t.Fatal("limit missing")
	}
}

func TestTOTPAndIndependentOriginCookies(t *testing.T) {
	key := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	if !VerifyTOTP(key, "287082", time.Unix(59, 0)) {
		t.Fatal("RFC 6238 vector")
	}
	if VerifyTOTP(key, "287082", time.Unix(200, 0)) || VerifyTOTP("AA", "000000", time.Now()) {
		t.Fatal("invalid TOTP accepted")
	}
	a, b := Gate{Origin: "http://127.0.0.1:17860"}, Gate{Origin: "http://127.0.0.1:17870"}
	if a.cookieName() == b.cookieName() {
		t.Fatal("local service cookies collide")
	}
}
