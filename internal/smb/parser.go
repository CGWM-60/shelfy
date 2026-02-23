package smb

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type smbStatusJSON struct {
	Sessions []struct {
		Username string `json:"username"`
		Machine  string `json:"machine"`
		IP       string `json:"ip"`
		Start    string `json:"start"`
	} `json:"sessions"`
}

func parseSambaStatusJSON(raw []byte) ([]Client, error) {
	var payload smbStatusJSON
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse smbstatus json: %w", err)
	}
	out := make([]Client, 0, len(payload.Sessions))
	for _, session := range payload.Sessions {
		client := Client{
			Username: strings.TrimSpace(session.Username),
			Machine:  strings.TrimSpace(session.Machine),
			Address:  strings.TrimSpace(session.IP),
		}
		if ts := parseSambaTime(session.Start); !ts.IsZero() {
			client.ConnectedAt = ts
		}
		if client.Address == "" && client.Machine == "" && client.Username == "" {
			continue
		}
		out = append(out, client)
	}
	return out, nil
}

func parseSambaTime(raw string) time.Time {
	value := strings.TrimSpace(raw)
	if value == "" {
		return time.Time{}
	}
	layoutCandidates := []string{time.RFC3339, "2006-01-02 15:04:05", time.RFC1123Z, time.RFC1123}
	for _, layout := range layoutCandidates {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed
		}
	}
	return time.Time{}
}
