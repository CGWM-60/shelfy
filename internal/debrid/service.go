package debrid

import (
	"context"
	"errors"
	"fmt"
	"time"

	"cgwm/shelfy/internal/domain"
	"cgwm/shelfy/internal/repo"
)

type Service struct {
	repo     repo.Repository
	registry *Registry
}

func NewService(r repo.Repository, registry *Registry) *Service {
	return &Service{repo: r, registry: registry}
}

type ProviderInfo struct {
	Name             string   `json:"name"`
	AuthType         AuthType `json:"authType"`
	SupportsPassword bool     `json:"supportsPassword"`
}

type AccountPatch struct {
	Label     *string `json:"label,omitempty"`
	IsActive  *bool   `json:"isActive,omitempty"`
	IsDefault *bool   `json:"isDefault,omitempty"`
}

type ResolveOptions struct {
	UseDebrid bool
	Provider  string
	AccountID uint
	Password  string
}

type ResolvedItem struct {
	SourceLink string `json:"sourceLink"`
	DirectLink string `json:"directLink"`
	FileName   string `json:"fileName"`
	Provider   string `json:"provider"`
}

func (s *Service) ListProviders() []ProviderInfo {
	providers := s.registry.List()
	out := make([]ProviderInfo, 0, len(providers))
	for _, p := range providers {
		_, supportsPassword := p.(PasswordAuthProvider)
		out = append(out, ProviderInfo{Name: p.Name(), AuthType: p.AuthType(), SupportsPassword: supportsPassword})
	}
	return out
}

func (s *Service) CreateAccount(ctx context.Context, provider, label, apiKey string, isActive, isDefault bool) (domain.DebridAccount, error) {
	p, err := s.registry.Get(provider)
	if err != nil {
		return domain.DebridAccount{}, err
	}
	if isDefault {
		_ = s.repo.DeactivateDebridDefaults(ctx, provider)
	}
	account := domain.DebridAccount{
		Provider:  provider,
		Label:     label,
		IsActive:  isActive,
		IsDefault: isDefault,
		AuthType:  domain.DebridAuthType(p.AuthType()),
		APIKey:    apiKey,
	}
	return s.repo.CreateDebridAccount(ctx, account)
}

func (s *Service) ListAccounts(ctx context.Context) ([]domain.DebridAccount, error) {
	return s.repo.ListDebridAccounts(ctx)
}

func (s *Service) PatchAccount(ctx context.Context, id uint, patch AccountPatch) (domain.DebridAccount, error) {
	account, err := s.repo.GetDebridAccount(ctx, id)
	if err != nil {
		return domain.DebridAccount{}, err
	}
	if patch.Label != nil {
		account.Label = *patch.Label
	}
	if patch.IsActive != nil {
		account.IsActive = *patch.IsActive
	}
	if patch.IsDefault != nil {
		account.IsDefault = *patch.IsDefault
		if *patch.IsDefault {
			_ = s.repo.DeactivateDebridDefaults(ctx, account.Provider)
		}
	}
	return s.repo.UpdateDebridAccount(ctx, account)
}

func (s *Service) DeleteAccount(ctx context.Context, id uint) error {
	return s.repo.DeleteDebridAccount(ctx, id)
}

func (s *Service) StartAuth(ctx context.Context, accountID uint) (AuthSession, error) {
	account, err := s.repo.GetDebridAccount(ctx, accountID)
	if err != nil {
		return AuthSession{}, err
	}
	provider, err := s.registry.Get(account.Provider)
	if err != nil {
		return AuthSession{}, err
	}
	return provider.StartAuth(ctx)
}

func (s *Service) PollAuth(ctx context.Context, accountID uint, session AuthSession) (bool, error) {
	account, err := s.repo.GetDebridAccount(ctx, accountID)
	if err != nil {
		return false, err
	}
	provider, err := s.registry.Get(account.Provider)
	if err != nil {
		return false, err
	}
	token, done, err := provider.PollAuth(ctx, session)
	if err != nil {
		return false, err
	}
	if !done {
		return false, nil
	}
	account.AccessToken = token.AccessToken
	account.RefreshToken = token.RefreshToken
	account.TokenExpiry = token.Expiry
	_, err = s.repo.UpdateDebridAccount(ctx, account)
	return true, err
}

func (s *Service) AuthWithPassword(ctx context.Context, accountID uint, username, password string) error {
	account, err := s.repo.GetDebridAccount(ctx, accountID)
	if err != nil {
		return err
	}
	provider, err := s.registry.Get(account.Provider)
	if err != nil {
		return err
	}
	passwordProvider, ok := provider.(PasswordAuthProvider)
	if !ok {
		return errors.New("provider does not support password auth")
	}
	token, err := passwordProvider.AuthWithPassword(ctx, username, password)
	if err != nil {
		return err
	}
	account.AccessToken = token.AccessToken
	account.RefreshToken = token.RefreshToken
	account.TokenExpiry = token.Expiry
	account.AuthType = domain.DebridAuthOAuth2Pass
	_, err = s.repo.UpdateDebridAccount(ctx, account)
	return err
}

func (s *Service) Status(ctx context.Context, accountID uint) (string, error) {
	account, err := s.repo.GetDebridAccount(ctx, accountID)
	if err != nil {
		return "", err
	}
	provider, err := s.registry.Get(account.Provider)
	if err != nil {
		return "", err
	}
	err = provider.ValidateToken(ctx, Token{AccessToken: account.AccessToken, RefreshToken: account.RefreshToken, Expiry: account.TokenExpiry})
	if err != nil {
		return "invalid", nil
	}
	return "ok", nil
}

func (s *Service) ResolveLink(ctx context.Context, link string, opts ResolveOptions) ([]ResolvedItem, error) {
	if !opts.UseDebrid {
		return []ResolvedItem{{SourceLink: link, DirectLink: link, FileName: ""}}, nil
	}
	account, err := s.selectAccount(ctx, opts)
	if err != nil {
		return nil, err
	}
	provider, err := s.registry.Get(account.Provider)
	if err != nil {
		return nil, err
	}

	var result DebridResult
	for attempt := 1; attempt <= 3; attempt++ {
		result, err = provider.Unrestrict(ctx, link, UnrestrictOptions{
			Password: opts.Password,
			Account: Account{
				ID:           account.ID,
				Provider:     account.Provider,
				Label:        account.Label,
				IsActive:     account.IsActive,
				IsDefault:    account.IsDefault,
				AuthType:     AuthType(account.AuthType),
				AccessToken:  account.AccessToken,
				RefreshToken: account.RefreshToken,
				TokenExpiry:  account.TokenExpiry,
				APIKey:       account.APIKey,
			},
		})
		if err == nil {
			break
		}
		if !isTemporary(err) || attempt == 3 {
			return nil, err
		}
		time.Sleep(time.Duration(attempt) * 100 * time.Millisecond)
	}

	if result.Type == "file" {
		direct := result.DirectURL
		name := ""
		if result.File != nil {
			name = result.File.Name
			if direct == "" {
				direct = result.File.URL
			}
		}
		if direct == "" {
			return nil, errors.New("missing debrid direct URL")
		}
		return []ResolvedItem{{SourceLink: link, DirectLink: direct, FileName: name, Provider: account.Provider}}, nil
	}
	if result.Type == "folder" && result.Folder != nil {
		out := make([]ResolvedItem, 0, len(result.Folder.Files))
		for _, f := range result.Folder.Files {
			out = append(out, ResolvedItem{SourceLink: link, DirectLink: f.URL, FileName: f.Name, Provider: account.Provider})
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported debrid result type %s", result.Type)
}

func (s *Service) selectAccount(ctx context.Context, opts ResolveOptions) (domain.DebridAccount, error) {
	accounts, err := s.repo.ListDebridAccounts(ctx)
	if err != nil {
		return domain.DebridAccount{}, err
	}
	active := make([]domain.DebridAccount, 0)
	for _, account := range accounts {
		if account.IsActive {
			active = append(active, account)
		}
	}
	if len(active) == 0 {
		return domain.DebridAccount{}, errors.New("no active debrid account")
	}
	if opts.AccountID > 0 {
		for _, a := range active {
			if a.ID == opts.AccountID {
				return a, nil
			}
		}
		return domain.DebridAccount{}, errors.New("selected debrid account not active")
	}
	if opts.Provider != "" {
		for _, a := range active {
			if a.Provider == opts.Provider && a.IsDefault {
				return a, nil
			}
		}
		for _, a := range active {
			if a.Provider == opts.Provider {
				return a, nil
			}
		}
		return domain.DebridAccount{}, fmt.Errorf("no active account for provider %s", opts.Provider)
	}
	for _, a := range active {
		if a.IsDefault {
			return a, nil
		}
	}
	return active[0], nil
}

type temporary interface {
	Temporary() bool
}

func isTemporary(err error) bool {
	te, ok := err.(temporary)
	return ok && te.Temporary()
}
