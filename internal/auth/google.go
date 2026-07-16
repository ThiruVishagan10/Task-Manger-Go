package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Google's OAuth 2.0 endpoints. These are stable and published as part of
// Google's OpenID Connect discovery document.
const (
	googleAuthEndpoint     = "https://accounts.google.com/o/oauth2/v2/auth"
	googleTokenEndpoint    = "https://oauth2.googleapis.com/token"
	googleUserinfoEndpoint = "https://openidconnect.googleapis.com/v1/userinfo"
)

// googleTimeout bounds each call out to Google, so a sign-in cannot hang a
// request forever on an unresponsive dependency.
const googleTimeout = 10 * time.Second

// errorBodyLimit caps how much of a failed response we read back for the log.
// Enough to identify the error, not enough for a hostile or broken endpoint to
// feed us an unbounded body.
const errorBodyLimit = 2 << 10

// GoogleIdentity is the part of a Google profile this application uses.
type GoogleIdentity struct {
	// Sub is Google's immutable ID for the account. It, not the email, is what
	// a local user is matched on: an email can be changed and reassigned within
	// a Workspace domain, whereas sub never changes.
	Sub string

	Email string
	Name  string

	// EmailVerified reports whether Google vouches for the address. It is false
	// for some Workspace domains, and must be checked before the address is
	// trusted to identify anyone.
	EmailVerified bool
}

// GoogleClient talks to Google's OAuth 2.0 endpoints.
type GoogleClient struct {
	clientID     string
	clientSecret string
	redirectURL  string
	http         *http.Client
}

// NewGoogleClient returns a client for the given OAuth credentials. redirectURL
// must exactly match one of the redirect URIs registered for the client in the
// Google Cloud console; Google rejects the request outright otherwise.
func NewGoogleClient(clientID, clientSecret, redirectURL string) *GoogleClient {
	return &GoogleClient{
		clientID:     clientID,
		clientSecret: clientSecret,
		redirectURL:  redirectURL,
		http:         &http.Client{Timeout: googleTimeout},
	}
}

// AuthCodeURL returns the Google URL to send the user to in order to sign in.
//
// state is echoed back to the callback and must be checked there. codeChallenge
// binds the eventual authorization code to this browser (see NewPKCEVerifier).
func (c *GoogleClient) AuthCodeURL(state, codeChallenge string) string {
	q := url.Values{
		"client_id":             {c.clientID},
		"redirect_uri":          {c.redirectURL},
		"response_type":         {"code"},
		"scope":                 {"openid email profile"},
		"state":                 {state},
		"code_challenge":        {codeChallenge},
		"code_challenge_method": {"S256"},

		// No refresh token: this app acts on the user's behalf only during
		// sign-in, so there is nothing to refresh and no long-lived Google
		// credential worth storing.
		"access_type": {"online"},

		// Always let the user pick an account rather than silently reusing
		// whichever one their browser is already signed in to.
		"prompt": {"select_account"},
	}

	return googleAuthEndpoint + "?" + q.Encode()
}

// Exchange trades an authorization code for the identity of the user who
// approved it. codeVerifier must be the value the code_challenge was derived
// from.
func (c *GoogleClient) Exchange(ctx context.Context, code, codeVerifier string) (GoogleIdentity, error) {
	accessToken, err := c.exchangeCode(ctx, code, codeVerifier)
	if err != nil {
		return GoogleIdentity{}, err
	}

	return c.fetchIdentity(ctx, accessToken)
}

// exchangeCode posts the authorization code to Google and returns the access
// token it grants.
func (c *GoogleClient) exchangeCode(ctx context.Context, code, codeVerifier string) (string, error) {
	form := url.Values{
		"code":          {code},
		"client_id":     {c.clientID},
		"client_secret": {c.clientSecret},
		"redirect_uri":  {c.redirectURL},
		"grant_type":    {"authorization_code"},
		"code_verifier": {codeVerifier},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, googleTokenEndpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("build token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchange code with google: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google token endpoint: %s: %s", resp.Status, readErrorBody(resp.Body))
	}

	var body struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("decode google token response: %w", err)
	}

	if body.AccessToken == "" {
		return "", fmt.Errorf("google token response carried no access token")
	}

	return body.AccessToken, nil
}

// fetchIdentity reads the profile of the user the access token belongs to.
//
// The profile is fetched from the userinfo endpoint rather than decoded out of
// the id_token that accompanies it. Both carry the same claims, but this
// response is trustworthy because of where it came from — a TLS connection to
// Google, in reply to a request holding our client secret — whereas an id_token
// is only trustworthy once its signature has been checked against Google's
// rotating public keys. Taking the identity from the wire keeps a JWT library,
// a key cache, and a class of verification bugs out of the project.
func (c *GoogleClient) fetchIdentity(ctx context.Context, accessToken string) (GoogleIdentity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, googleUserinfoEndpoint, nil)
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("build userinfo request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return GoogleIdentity{}, fmt.Errorf("fetch google profile: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GoogleIdentity{}, fmt.Errorf("google userinfo endpoint: %s: %s", resp.Status, readErrorBody(resp.Body))
	}

	var body struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return GoogleIdentity{}, fmt.Errorf("decode google profile: %w", err)
	}

	if body.Sub == "" || body.Email == "" {
		return GoogleIdentity{}, fmt.Errorf("google profile is missing a subject or email")
	}

	return GoogleIdentity{
		Sub:           body.Sub,
		Email:         body.Email,
		Name:          body.Name,
		EmailVerified: body.EmailVerified,
	}, nil
}

// NewPKCEVerifier returns a fresh PKCE code verifier.
//
// The verifier stays with the browser that started the sign-in and is sent only
// at the final exchange; only its hash travels through Google. An authorization
// code intercepted in transit — out of a redirect URL, a proxy log, or browser
// history — is therefore useless without it.
//
// Base64url over 32 bytes yields 43 characters drawn from the unreserved set,
// which is exactly what RFC 7636 requires of a verifier.
func NewPKCEVerifier() (string, error) {
	return newToken()
}

// NewState returns a fresh OAuth state value.
//
// It is echoed back by Google to the callback, where it is compared against a
// cookie set here. A callback forged by another site carries no such cookie, so
// it cannot produce a match — which is what stops an attacker from having a
// victim's browser complete a sign-in into the attacker's account.
func NewState() (string, error) {
	return newToken()
}

// PKCEChallenge returns the S256 challenge to send to Google for a verifier.
func PKCEChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))

	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// readErrorBody returns a bounded snippet of an error response for logging.
func readErrorBody(r io.Reader) string {
	body, err := io.ReadAll(io.LimitReader(r, errorBodyLimit))
	if err != nil {
		return "<unreadable>"
	}

	return strings.TrimSpace(string(body))
}
