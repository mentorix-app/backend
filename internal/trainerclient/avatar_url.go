package trainerclient

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
)

const avatarURLTTL = 24 * time.Hour

func BuildAvatarURL(clientUserID uuid.UUID, avatarFilePath, secret string) string {
	if avatarFilePath == "" || secret == "" {
		return ""
	}
	exp := time.Now().Add(avatarURLTTL).Unix()
	sig := signAvatarURL(secret, clientUserID, exp)
	return fmt.Sprintf("/trainer/clients/%s/avatar?exp=%d&sig=%s", clientUserID, exp, sig)
}

func VerifyAvatarURL(secret string, clientUserID uuid.UUID, exp int64, sig string) bool {
	if secret == "" || sig == "" || exp <= 0 {
		return false
	}
	if time.Now().Unix() > exp {
		return false
	}
	expected := signAvatarURL(secret, clientUserID, exp)
	return hmac.Equal([]byte(expected), []byte(sig))
}

func signAvatarURL(secret string, clientUserID uuid.UUID, exp int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("avatar:"))
	_, _ = mac.Write([]byte(clientUserID.String()))
	_, _ = mac.Write([]byte(":"))
	_, _ = mac.Write([]byte(strconv.FormatInt(exp, 10)))
	return hex.EncodeToString(mac.Sum(nil))
}
