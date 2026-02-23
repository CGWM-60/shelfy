package fake

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"cgwm/shelfy/internal/debrid"
)

type Provider struct {
	mu         sync.Mutex
	retryCount map[string]int
	pollCount  map[string]int
}

func New() *Provider {
	return &Provider{retryCount: map[string]int{}, pollCount: map[string]int{}}
}

func (p *Provider) Name() string {
	return "fake"
}

func (p *Provider) AuthType() debrid.AuthType {
	return debrid.AuthTypeOAuth2Device
}

func (p *Provider) StartAuth(ctx context.Context) (debrid.AuthSession, error) {
	return debrid.AuthSession{SessionID: "fake-session", DeviceCode: "fake-device-code", UserCode: "ABCD-1234", VerificationURI: "https://fake.local/device", IntervalSec: 1, ExpiresIn: 600}, nil
}

func (p *Provider) PollAuth(ctx context.Context, session debrid.AuthSession) (debrid.Token, bool, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	key := session.SessionID
	if key == "" {
		key = session.DeviceCode
	}
	p.pollCount[key]++
	if p.pollCount[key] < 2 {
		return debrid.Token{}, false, nil
	}
	return debrid.Token{AccessToken: "fake-token", RefreshToken: "fake-refresh"}, true, nil
}

func (p *Provider) AuthWithPassword(ctx context.Context, username, password string) (debrid.Token, error) {
	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return debrid.Token{}, errors.New("username and password are required")
	}
	return debrid.Token{AccessToken: "fake-pass-token", RefreshToken: "fake-pass-refresh"}, nil
}

func (p *Provider) ValidateToken(ctx context.Context, token debrid.Token) error {
	if token.AccessToken == "" {
		return errors.New("missing token")
	}
	return nil
}

func (p *Provider) Unrestrict(ctx context.Context, link string, opts debrid.UnrestrictOptions) (debrid.DebridResult, error) {
	if strings.HasPrefix(link, "fake:direct:") {
		direct := strings.TrimPrefix(link, "fake:direct:")
		return debrid.DebridResult{Provider: p.Name(), Type: "file", DirectURL: direct, File: &debrid.FileInfo{ID: "f-1", Name: deriveName(direct), URL: direct}}, nil
	}
	if strings.HasPrefix(link, "fake:folder:") {
		payload := strings.TrimPrefix(link, "fake:folder:")
		parts := strings.SplitN(payload, ":", 2)
		if len(parts) != 2 {
			return debrid.DebridResult{}, errors.New("invalid fake folder link")
		}
		n, err := strconv.Atoi(parts[0])
		if err != nil || n <= 0 {
			return debrid.DebridResult{}, errors.New("invalid fake folder count")
		}
		base := parts[1]
		files := make([]debrid.FileInfo, 0, n)
		for i := 1; i <= n; i++ {
			url := fmt.Sprintf("%s/file-%d.bin", strings.TrimRight(base, "/"), i)
			files = append(files, debrid.FileInfo{ID: fmt.Sprintf("file-%d", i), Name: fmt.Sprintf("file-%d.bin", i), URL: url, Size: int64(1024 * i)})
		}
		return debrid.DebridResult{Provider: p.Name(), Type: "folder", Folder: &debrid.FolderInfo{Name: "fake-folder", Files: files}}, nil
	}
	if strings.HasPrefix(link, "fake:retry:") {
		direct := strings.TrimPrefix(link, "fake:retry:")
		p.mu.Lock()
		p.retryCount[link]++
		count := p.retryCount[link]
		p.mu.Unlock()
		if count == 1 {
			return debrid.DebridResult{}, transientError{err: errors.New("temporary upstream error")}
		}
		return debrid.DebridResult{Provider: p.Name(), Type: "file", DirectURL: direct, File: &debrid.FileInfo{ID: "retry-1", Name: deriveName(direct), URL: direct}}, nil
	}
	return debrid.DebridResult{}, errors.New("fake provider unsupported link")
}

func (p *Provider) ListFolder(ctx context.Context, link string) (debrid.FolderInfo, error) {
	result, err := p.Unrestrict(ctx, link, debrid.UnrestrictOptions{})
	if err != nil {
		return debrid.FolderInfo{}, err
	}
	if result.Folder == nil {
		return debrid.FolderInfo{}, errors.New("not a folder")
	}
	return *result.Folder, nil
}

func (p *Provider) GetStreamURL(ctx context.Context, fileID string) (string, error) {
	return "", errors.New("not implemented in fake provider")
}

type transientError struct {
	err error
}

func (e transientError) Error() string {
	return e.err.Error()
}

func (e transientError) Temporary() bool {
	return true
}

func deriveName(raw string) string {
	parts := strings.Split(strings.TrimRight(raw, "/"), "/")
	if len(parts) == 0 {
		return "file.bin"
	}
	if parts[len(parts)-1] == "" {
		return "file.bin"
	}
	return parts[len(parts)-1]
}
