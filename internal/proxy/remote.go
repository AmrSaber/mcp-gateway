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
func remoteTransport(ctx context.Context, spec ServerSpec, env []string) (mcp.Transport, error) {
	headers, err := interpolateMap(ctx, spec.Headers, env)
	if err != nil {
		return nil, fmt.Errorf("resolving headers: %w", err)
	}

	httpClient := &http.Client{}
	if len(headers) > 0 {
		httpClient.Transport = &headerRoundTripper{Headers: headers, Base: http.DefaultTransport}
	}

	transport := &mcp.StreamableClientTransport{
		Endpoint:   spec.URL,
		HTTPClient: httpClient,
	}

	if spec.OAuth != nil {
		clientID, err := interpolate(ctx, spec.OAuth.ClientID, env)
		if err != nil {
			return nil, fmt.Errorf("resolving oauth.clientId: %w", err)
		}
		clientSecret, err := interpolate(ctx, spec.OAuth.ClientSecret, env)
		if err != nil {
			return nil, fmt.Errorf("resolving oauth.clientSecret: %w", err)
		}

		handler, err := extauth.NewClientCredentialsHandler(&extauth.ClientCredentialsHandlerConfig{
			Credentials: &oauthex.ClientCredentials{
				ClientID:         clientID,
				ClientSecretAuth: &oauthex.ClientSecretAuth{ClientSecret: clientSecret},
			},
			HTTPClient: httpClient,
		})
		if err != nil {
			return nil, err
		}
		transport.OAuthHandler = auth.OAuthHandler(handler)
	}

	return transport, nil
}

// headerRoundTripper adds a fixed set of headers to every outgoing request.
type headerRoundTripper struct {
	Headers map[string]string
	Base    http.RoundTripper
}

func (rt *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// Clone so we never mutate a request the caller may reuse.
	req = req.Clone(req.Context())
	for key, val := range rt.Headers {
		req.Header.Set(key, val)
	}
	return rt.Base.RoundTrip(req)
}
