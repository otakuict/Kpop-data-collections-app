package gallery

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

const adminSessionLifetime = 24 * time.Hour
const adminJWTHeader = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9" // {"alg":"HS256","typ":"JWT"}

type adminClaims struct {
	Subject  string `json:"sub"`
	Audience string `json:"aud"`
	Issued  int64  `json:"iat"`
	Expires int64  `json:"exp"`
}

func adminBearer(header string) string {
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimPrefix(header, "Bearer ")
}

func validAdminToken(supplied, token string) bool {
	return len(token) >= 24 && subtle.ConstantTimeCompare([]byte(supplied), []byte(token)) == 1
}

func adminSigningKey(token string) [32]byte {
	return sha256.Sum256([]byte("bias-archive-admin-jwt-v1:" + token))
}

func signAdminJWT(message, token string) []byte {
	key := adminSigningKey(token)
	mac := hmac.New(sha256.New, key[:])
	mac.Write([]byte(message))
	return mac.Sum(nil)
}

func issueAdminJWT(token string, now time.Time) string {
	claims, _ := json.Marshal(adminClaims{Subject: "administrator", Audience: "backoffice", Issued: now.Unix(), Expires: now.Add(adminSessionLifetime).Unix()})
	message := adminJWTHeader + "." + base64.RawURLEncoding.EncodeToString(claims)
	return message + "." + base64.RawURLEncoding.EncodeToString(signAdminJWT(message, token))
}

func validAdminJWT(jwt, token string, now time.Time) bool {
	if len(token) < 24 || len(jwt) > 2048 {
		return false
	}
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 || parts[0] != adminJWTHeader {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(signature, signAdminJWT(parts[0]+"."+parts[1], token)) {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	var claims adminClaims
	if json.Unmarshal(payload, &claims) != nil {
		return false
	}
	current := now.Unix()
	return claims.Subject == "administrator" && claims.Audience == "backoffice" &&
		claims.Issued <= current && claims.Issued > current-int64(adminSessionLifetime.Seconds()) &&
		claims.Expires > current && claims.Expires == claims.Issued+int64(adminSessionLifetime.Seconds())
}
