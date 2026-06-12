package auth

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const castTokenTTL = 4 * time.Hour

// CastClaims is the JWT payload for a Chromecast cast token.
// It is scoped to a single media item to prevent token reuse across items.
type CastClaims struct {
	MediaID string `json:"mid"`
	jwt.RegisteredClaims
}

// IssueCastToken creates a short-lived HS256 JWT authorizing Chromecast
// playback of the given media item. The token is valid for 4 hours and
// cannot be used to access any other media item.
func IssueCastToken(secret, mediaID string) (string, error) {
	claims := CastClaims{
		MediaID: mediaID,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(castTokenTTL)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		return "", fmt.Errorf("signing cast token: %w", err)
	}

	return signed, nil
}

// ValidateCastToken parses and validates a cast token, confirming that it
// is unexpired and scoped to the expected media item.
func ValidateCastToken(secret, tokenStr, expectedMediaID string) error {
	token, err := jwt.ParseWithClaims(tokenStr, &CastClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return fmt.Errorf("parsing cast token: %w", err)
	}

	claims, ok := token.Claims.(*CastClaims)
	if !ok || !token.Valid {
		return fmt.Errorf("invalid cast token claims")
	}

	if claims.MediaID != expectedMediaID {
		return fmt.Errorf("cast token not valid for this media item")
	}

	return nil
}
