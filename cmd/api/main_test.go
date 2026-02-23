package main

import "testing"

func TestGuessUIBaseURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		dlnaBaseURL string
		apiBaseURL  string
		want        string
	}{
		{
			name:        "prefer dlna base url when provided",
			dlnaBaseURL: "http://192.168.1.200:8080/",
			apiBaseURL:  "http://127.0.0.1:18080",
			want:        "http://192.168.1.200:8080",
		},
		{
			name:       "map api port 18080 to ui 8080",
			apiBaseURL: "http://127.0.0.1:18080",
			want:       "http://127.0.0.1:8080",
		},
		{
			name:       "keep ui url when api already on 8080",
			apiBaseURL: "http://192.168.1.20:8080",
			want:       "http://192.168.1.20:8080",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := guessUIBaseURL(tc.dlnaBaseURL, tc.apiBaseURL)
			if got != tc.want {
				t.Fatalf("guessUIBaseURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRedactedDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		driver string
		dsn    string
		want   string
	}{
		{
			name:   "mysql dsn password redacted",
			driver: "mysql",
			dsn:    "user:secret@tcp(db:3306)/shelfy",
			want:   "user:***@tcp(db:3306)/shelfy",
		},
		{
			name:   "url dsn password redacted",
			driver: "postgres",
			dsn:    "postgres://user:secret@db:5432/shelfy",
			want:   "postgres://user:%2A%2A%2A@db:5432/shelfy",
		},
		{
			name:   "sqlite dsn unchanged",
			driver: "sqlite",
			dsn:    "/data/shelfy.db",
			want:   "/data/shelfy.db",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := redactedDSN(tc.driver, tc.dsn)
			if got != tc.want {
				t.Fatalf("redactedDSN() = %q, want %q", got, tc.want)
			}
		})
	}
}
