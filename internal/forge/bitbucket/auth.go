package bitbucket

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"go.abhg.dev/gs/internal/forge"
	"go.abhg.dev/gs/internal/secret"
	"go.abhg.dev/gs/internal/silog"
	"go.abhg.dev/gs/internal/ui"
	"go.abhg.dev/gs/internal/xec"
)

// AuthType identifies the authentication method used.
type AuthType int

const (
	// AuthTypeAppPassword indicates authentication via App Password.
	AuthTypeAppPassword AuthType = iota

	// AuthTypeGCM indicates authentication via git-credential-manager.
	// GCM stores OAuth tokens obtained through browser-based authentication.
	AuthTypeGCM

	// AuthTypeEnvironmentVariable indicates authentication via environment variable.
	// This is set to 100 to distinguish from user-selected auth types.
	AuthTypeEnvironmentVariable AuthType = 100
)

// AuthenticationToken defines the token returned by the Bitbucket forge.
type AuthenticationToken struct {
	forge.AuthenticationToken

	// AuthType specifies the authentication method used.
	AuthType AuthType `json:"auth_type"`

	// AccessToken is the Bitbucket App Password.
	AccessToken string `json:"access_token,omitempty"`

	// Email stores the Bitbucket username for App Password authentication.
	// Bitbucket uses username:app_password for Basic auth.
	// Named "Email" for JSON backwards compatibility.
	Email string `json:"email,omitempty"`
}

var _ forge.AuthenticationToken = (*AuthenticationToken)(nil)

// authMethod identifies a user-selectable authentication method.
type authMethod int

const (
	authMethodGCM authMethod = iota
	authMethodAppPassword
)

// AuthenticationFlow prompts the user to authenticate with Bitbucket.
// This rejects the request if the user is already authenticated
// with a BITBUCKET_TOKEN environment variable.
func (f *Forge) AuthenticationFlow(
	ctx context.Context,
	view ui.View,
) (forge.AuthenticationToken, error) {
	log := f.logger()

	if f.Options.Token != "" {
		log.Error("Already authenticated with BITBUCKET_TOKEN.")
		log.Error("Unset BITBUCKET_TOKEN to login with a different method.")
		return nil, errors.New("already authenticated")
	}

	method, err := f.selectAuthMethod(view)
	if err != nil {
		return nil, fmt.Errorf("select auth method: %w", err)
	}

	switch method {
	case authMethodGCM:
		return f.gcmAuth(log)
	case authMethodAppPassword:
		return f.appPasswordAuth(ctx, view)
	default:
		return nil, fmt.Errorf("unknown auth method: %d", method)
	}
}

func (f *Forge) selectAuthMethod(view ui.View) (authMethod, error) {
	methods := []ui.ListItem[authMethod]{
		{
			Title:       "Git Credential Manager",
			Description: gcmAuthDescription,
			Value:       authMethodGCM,
		},
		{
			Title:       "App Password",
			Description: appPasswordAuthDescription,
			Value:       authMethodAppPassword,
		},
	}

	var method authMethod
	err := ui.Run(view,
		ui.NewList[authMethod]().
			WithTitle("Select an authentication method").
			WithItems(methods...).
			WithValue(&method),
	)
	return method, err
}

func gcmAuthDescription(bool) string {
	return "Use OAuth credentials from git-credential-manager.\n" +
		"You must have GCM installed and already authenticated."
}

func appPasswordAuthDescription(bool) string {
	return "Enter an App Password manually.\n" +
		"Create one at https://bitbucket.org/account/settings/app-passwords/"
}

func (f *Forge) gcmAuth(log *silog.Logger) (*AuthenticationToken, error) {
	token, err := f.loadGCMCredentials()
	if err != nil {
		log.Error("Could not load credentials from git-credential-manager.")
		log.Error("Ensure GCM is installed and you have authenticated to Bitbucket.")
		return nil, fmt.Errorf("load GCM credentials: %w", err)
	}

	log.Info("Successfully loaded credentials from git-credential-manager.")
	return token, nil
}

func (f *Forge) appPasswordAuth(_ context.Context, view ui.View) (*AuthenticationToken, error) {
	f.logger().Info("Bitbucket Cloud uses API tokens for authentication.")
	f.logger().Info("Create one at: https://bitbucket.org/account/settings/api-tokens/")
	f.logger().Info("Required scopes: read:repository, write:repository, read:pullrequest, write:pullrequest")

	email, err := promptRequired(view,
		"Enter Atlassian account email", "email is required")
	if err != nil {
		return nil, fmt.Errorf("prompt for email: %w", err)
	}

	token, err := promptRequired(view, "Enter API token", "API token is required")
	if err != nil {
		return nil, fmt.Errorf("prompt for API token: %w", err)
	}

	return &AuthenticationToken{
		AuthType:    AuthTypeAppPassword,
		AccessToken: token,
		Email:       email, // Bitbucket uses email:token for Basic auth
	}, nil
}

func promptRequired(view ui.View, title, errMsg string) (string, error) {
	var value string
	err := ui.Run(view, ui.NewInput().
		WithTitle(title).
		WithValidate(requiredValidator(errMsg)).
		WithValue(&value),
	)
	return value, err
}

func requiredValidator(errMsg string) func(string) error {
	return func(input string) error {
		if strings.TrimSpace(input) == "" {
			return errors.New(errMsg)
		}
		return nil
	}
}

// SaveAuthenticationToken saves the given authentication token to the stash.
func (f *Forge) SaveAuthenticationToken(
	stash secret.Stash,
	t forge.AuthenticationToken,
) error {
	bbt := t.(*AuthenticationToken)

	// If the user has set BITBUCKET_TOKEN, we should not save it to the stash.
	if f.Options.Token != "" && f.Options.Token == bbt.AccessToken {
		return nil
	}

	data, err := json.Marshal(bbt)
	if err != nil {
		return fmt.Errorf("marshal token: %w", err)
	}

	return stash.SaveSecret(f.URL(), "token", string(data))
}

// LoadAuthenticationToken loads the authentication token from the stash.
// Priority order:
//  1. Environment variable (BITBUCKET_TOKEN)
//  2. Stored token in secret stash
//  3. git-credential-manager (GCM)
func (f *Forge) LoadAuthenticationToken(stash secret.Stash) (forge.AuthenticationToken, error) {
	// Environment variable takes highest precedence.
	if f.Options.Token != "" {
		return &AuthenticationToken{
			AuthType:    AuthTypeEnvironmentVariable,
			AccessToken: f.Options.Token,
		}, nil
	}

	// Try stored token next.
	if token, err := f.loadStoredToken(stash); err == nil {
		return token, nil
	}

	// Fall back to git-credential-manager.
	if token, err := f.loadGCMCredentials(); err == nil {
		f.logger().Debug("Using credentials from git-credential-manager")
		return token, nil
	}

	return nil, errors.New("no authentication token available")
}

func (f *Forge) loadStoredToken(stash secret.Stash) (*AuthenticationToken, error) {
	data, err := stash.LoadSecret(f.URL(), "token")
	if err != nil {
		return nil, err
	}

	var token AuthenticationToken
	if err := json.Unmarshal([]byte(data), &token); err != nil {
		return nil, err
	}
	return &token, nil
}

// ClearAuthenticationToken removes the authentication token from the stash.
func (f *Forge) ClearAuthenticationToken(stash secret.Stash) error {
	return stash.DeleteSecret(f.URL(), "token")
}

// loadGCMCredentials attempts to load OAuth credentials from git-credential-manager.
// Returns nil if GCM credentials are not available.
func (f *Forge) loadGCMCredentials() (*AuthenticationToken, error) {
	host := extractHost(f.URL())
	input := fmt.Sprintf("protocol=https\nhost=%s\n\n", host)

	ctx := context.Background()
	output, err := xec.Command(ctx, nil, "git", "credential", "fill").
		WithStdinString(input).
		Output()
	if err != nil {
		return nil, fmt.Errorf("git credential fill: %w", err)
	}

	return parseCredentialOutput(output)
}

// parseCredentialOutput parses the output of `git credential fill`.
// The format is key=value pairs, one per line.
func parseCredentialOutput(output []byte) (*AuthenticationToken, error) {
	var username, password string

	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		line := scanner.Text()
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		switch key {
		case "username":
			username = value
		case "password":
			password = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse credential output: %w", err)
	}

	if password == "" {
		return nil, errors.New("no password in credential output")
	}

	return &AuthenticationToken{
		AuthType:    AuthTypeGCM,
		AccessToken: password,
		Email:       username,
	}, nil
}

// extractHost extracts the host from a URL.
func extractHost(rawURL string) string {
	// Remove protocol prefix.
	host := rawURL
	if idx := strings.Index(host, "://"); idx != -1 {
		host = host[idx+3:]
	}

	// Remove path suffix.
	if idx := strings.Index(host, "/"); idx != -1 {
		host = host[:idx]
	}
	return host
}
