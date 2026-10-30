package middleware

import (
	"context"
	"fmt"
	"heart-rate-server/internal/config"
	"heart-rate-server/internal/models"
	"heart-rate-server/internal/utils"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/securecookie"
)

type SecureCookie struct {
	s *securecookie.SecureCookie
}

func NewSecureCookie(hashKey, blockKey []byte) *SecureCookie {
	return &SecureCookie{
		s: securecookie.New(hashKey, blockKey),
	}
}

func (sc *SecureCookie) SetAuthCookie(w http.ResponseWriter, authInfo models.AuthInfo) error {
	encoded, err := sc.s.Encode("heart-rate-auth", authInfo)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "heart-rate-auth",
		Value:    encoded,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  authInfo.Expires,
	})
	return nil
}

func (sc *SecureCookie) GetAuthInfo(r *http.Request) (*models.AuthInfo, error) {
	cookie, err := r.Cookie("heart-rate-auth")
	if err != nil {
		return nil, err
	}

	var authInfo models.AuthInfo
	if err := sc.s.Decode("heart-rate-auth", cookie.Value, &authInfo); err != nil {
		return nil, err
	}

	if time.Now().After(authInfo.Expires) {
		return nil, fmt.Errorf("cookie expired")
	}

	return &authInfo, nil
}

func (sc *SecureCookie) ClearAuthCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     "heart-rate-auth",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Unix(0, 0),
	})
}

func AuthMiddleware(sc *SecureCookie, config *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authInfo, err := sc.GetAuthInfo(r)
			if err != nil {
				utils.SendError(w, http.StatusUnauthorized, err, "Unauthorized")
				return
			}

			// 自动续期：如果 Cookie 剩余有效期小于总有效期的一半，则续期
			if time.Until(authInfo.Expires) < config.TokenExpiry/2 {
				authInfo.Expires = time.Now().Add(config.TokenExpiry)
				if err := sc.SetAuthCookie(w, *authInfo); err != nil {
					log.Printf("Failed to renew cookie: %v", err)
				}
			}

			ctx := context.WithValue(r.Context(), "authInfo", authInfo)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
