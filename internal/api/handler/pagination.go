package handler

import (
	"encoding/base64"
	"encoding/json"
)

type PageToken struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

func encodePageToken(offset, limit int) string {
	token := PageToken{Offset: offset, Limit: limit}
	b, err := json.Marshal(token)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func decodePageToken(tokenStr string) (int, int, error) {
	b, err := base64.RawURLEncoding.DecodeString(tokenStr)
	if err != nil {
		return 0, 0, err
	}
	var token PageToken
	if err := json.Unmarshal(b, &token); err != nil {
		return 0, 0, err
	}
	return token.Offset, token.Limit, nil
}
