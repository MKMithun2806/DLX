package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"net/http"
)

const csrfCookieName = "vd_csrf"
const csrfHeaderName = "X-CSRF-Token"
const csrfFormField = "csrf_token"

func generateToken() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// CSRF implements the double-submit cookie pattern: a random token is set
// as a cookie, and every state-changing request (POST/PUT/DELETE/PATCH)
// must echo it back via header or form field. GET/HEAD/OPTIONS are exempt
// and also ensure the cookie is (re)issued.
func CSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(csrfCookieName)
		token := ""
		if err == nil {
			token = cookie.Value
		}
		if token == "" {
			token = generateToken()
			http.SetCookie(w, &http.Cookie{
				Name:     csrfCookieName,
				Value:    token,
				Path:     "/",
				HttpOnly: false, // must be readable by JS to echo back in headers
				SameSite: http.SameSiteLaxMode,
			})
		}

		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			next.ServeHTTP(w, r)
			return
		}

		// API clients using bearer-style auth could be exempted here in the
		// future; for now all mutating requests must present the token.
		sent := r.Header.Get(csrfHeaderName)
		if sent == "" {
			sent = r.FormValue(csrfFormField)
		}
		if sent == "" || sent != token {
			http.Error(w, "invalid or missing CSRF token", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}
