package proxy

import (
	"context"
	"fmt"
	"net/http"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/auth/extauth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
)

// remoteTransport builds a streamable-HTTP transport for a remote server. It
// interpolates {env:...}/{cmd:...} in the header values and OAuth credentials
// against Env (the merged environment prepared by transportFor). Static Headers
// are injected on every request via a wrapping RoundTripper, and OAuth (when
// configured) is handled by a client-credentials handler that discovers the
// token endpoint from server metadata and refreshes tokens automatically. Only
// pre-registered client-credentials OAuth is supported — the interactive
// authorization-code flow is future work.
func remoteTransport(Ctx context.Context, Spec ServerSpec, Env []string) (mcp.Transport, error) {
	Headers, Err := interpolateMap(Ctx, Spec.Headers, Env)
	if Err != nil {
		return nil, fmt.Errorf("resolving headers: %w", Err)
	}

	HTTPClient := &http.Client{}
	if len(Headers) > 0 {
		HTTPClient.Transport = &headerRoundTripper{Headers: Headers, Base: http.DefaultTransport}
	}

	Transport := &mcp.StreamableClientTransport{
		Endpoint:   Spec.URL,
		HTTPClient: HTTPClient,
	}

	if Spec.OAuth != nil {
		ClientID, Err := interpolate(Ctx, Spec.OAuth.ClientID, Env)
		if Err != nil {
			return nil, fmt.Errorf("resolving oauth.clientId: %w", Err)
		}
		ClientSecret, Err := interpolate(Ctx, Spec.OAuth.ClientSecret, Env)
		if Err != nil {
			return nil, fmt.Errorf("resolving oauth.clientSecret: %w", Err)
		}

		Handler, Err := extauth.NewClientCredentialsHandler(&extauth.ClientCredentialsHandlerConfig{
			Credentials: &oauthex.ClientCredentials{
				ClientID:         ClientID,
				ClientSecretAuth: &oauthex.ClientSecretAuth{ClientSecret: ClientSecret},
			},
			HTTPClient: HTTPClient,
		})
		if Err != nil {
			return nil, Err
		}
		Transport.OAuthHandler = auth.OAuthHandler(Handler)
	}

	return Transport, nil
}

// headerRoundTripper adds a fixed set of headers to every outgoing request.
type headerRoundTripper struct {
	Headers map[string]string
	Base    http.RoundTripper
}

func (Rt *headerRoundTripper) RoundTrip(Req *http.Request) (*http.Response, error) {
	// Clone so we never mutate a request the caller may reuse.
	Req = Req.Clone(Req.Context())
	for Key, Val := range Rt.Headers {
		Req.Header.Set(Key, Val)
	}
	return Rt.Base.RoundTrip(Req)
}
