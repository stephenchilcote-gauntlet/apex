package middleware

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const sessionCookieName = "apex_session"

// UIAuth returns middleware that enforces session-cookie authentication on UI routes.
// If username is empty the middleware is a no-op (dev / test mode).
func UIAuth(username, password, secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		if username == "" {
			return next // auth disabled
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			cookie, err := r.Cookie(sessionCookieName)
			if err != nil || !validSession(cookie.Value, username, secret) {
				redirectTo := r.URL.RequestURI()
				http.Redirect(w, r, "/ui/login?next="+url.QueryEscape(redirectTo), http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// UILoginHandler serves and processes the login form.
func UILoginHandler(username, password, secret string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			u := r.FormValue("username")
			p := r.FormValue("password")
			uOK := subtle.ConstantTimeCompare([]byte(u), []byte(username)) == 1
			pOK := subtle.ConstantTimeCompare([]byte(p), []byte(password)) == 1
			if uOK && pOK {
				http.SetCookie(w, &http.Cookie{
					Name:     sessionCookieName,
					Value:    signSession(username, secret),
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
					Expires:  time.Now().Add(8 * time.Hour),
				})
				next := r.FormValue("next")
				if next == "" || !strings.HasPrefix(next, "/ui/") {
					next = "/ui/simulate"
				}
				http.Redirect(w, r, next, http.StatusSeeOther)
				return
			}
			renderLogin(w, "Invalid username or password.")
			return
		}
		renderLogin(w, "")
	}
}

// signSession returns "<username>:<hmac>" for the given username.
func signSession(username, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(username))
	return fmt.Sprintf("%s:%s", username, hex.EncodeToString(mac.Sum(nil)))
}

// validSession returns true if the cookie value is a valid signed session for username.
func validSession(cookieValue, username, secret string) bool {
	parts := strings.SplitN(cookieValue, ":", 2)
	if len(parts) != 2 {
		return false
	}
	expected := signSession(username, secret)
	return subtle.ConstantTimeCompare([]byte(cookieValue), []byte(expected)) == 1
}

func renderLogin(w http.ResponseWriter, errMsg string) {
	errHTML := ""
	if errMsg != "" {
		errHTML = fmt.Sprintf(`<p style="color:red;margin:0 0 12px">%s</p>`, errMsg)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><html><head><title>Login — Apex</title>
<style>body{font-family:sans-serif;display:flex;justify-content:center;align-items:center;min-height:100vh;margin:0;background:#f4f4f5}
.box{background:#fff;padding:2rem;border-radius:8px;box-shadow:0 2px 8px rgba(0,0,0,.15);width:320px}
h2{margin:0 0 1.5rem;font-size:1.25rem}label{display:block;margin-bottom:.5rem;font-size:.875rem;font-weight:600}
input{width:100%%;box-sizing:border-box;padding:.5rem .75rem;border:1px solid #d1d5db;border-radius:4px;margin-bottom:1rem;font-size:1rem}
button{width:100%%;padding:.6rem;background:#2563eb;color:#fff;border:none;border-radius:4px;font-size:1rem;cursor:pointer}
button:hover{background:#1d4ed8}</style></head>
<body><div class="box"><h2>Apex Operator Login</h2>%s
<form method="POST"><label>Username<input name="username" type="text" required autofocus></label>
<label>Password<input name="password" type="password" required></label>
<button type="submit">Sign in</button></form></div></body></html>`, errHTML)
}
