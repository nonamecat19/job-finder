package retrieval

import (
	"context"
	"fmt"
	"io"
	stdhttp "net/http"
	"net/url"

	http "github.com/bogdanfinn/fhttp"
	tlsclient "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

type directRung struct {
	identity *BrowserIdentity
	store    *StateStore
}

func newDirectRung(identity *BrowserIdentity, store *StateStore) *directRung {
	return &directRung{identity: identity, store: store}
}

func (d *directRung) Fetch(ctx context.Context, rawURL string, headers map[string]string) (string, int, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", 0, fmt.Errorf("direct: parse url: %w", err)
	}
	host := u.Host

	cli, err := d.clientFor(ctx, host)
	if err != nil {
		return "", 0, fmt.Errorf("direct: init client: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return "", 0, err
	}

	for _, h := range d.identity.Headers {
		if h[0] == "Sec-Fetch-Site" || h[0] == "Sec-Fetch-Mode" || h[0] == "Sec-Fetch-Dest" {
			continue
		}
		if !headerExists(headers, h[0]) {
			req.Header.Set(h[0], h[1])
		}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Dest", "document")

	res, err := cli.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("direct: fetch %s: %w", rawURL, err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(io.LimitReader(res.Body, 10*1024*1024))
	if err != nil {
		return "", res.StatusCode, err
	}

	if d.store != nil && res.StatusCode < 400 {
		cookies := cli.GetCookies(res.Request.URL)
		if len(cookies) > 0 {
			stdCookies := make([]*stdhttp.Cookie, len(cookies))
			for i, c := range cookies {
				stdCookies[i] = &stdhttp.Cookie{
					Name:     c.Name,
					Value:    c.Value,
					Path:     c.Path,
					Domain:   c.Domain,
					Expires:  c.Expires,
					Secure:   c.Secure,
					HttpOnly: c.HttpOnly,
				}
			}
			_ = d.store.SaveCookies(ctx, host, stdCookies)
		}
	}

	return string(body), res.StatusCode, nil
}

func (d *directRung) clientFor(ctx context.Context, host string) (tlsclient.HttpClient, error) {
	profile, err := tlsProfileFor(d.identity.TLSProfileID)
	if err != nil {
		return nil, err
	}

	options := []tlsclient.HttpClientOption{
		tlsclient.WithTimeout(30),
		tlsclient.WithClientProfile(profile),
		tlsclient.WithRandomTLSExtensionOrder(),
		tlsclient.WithNotFollowRedirects(),
	}

	if d.store != nil {
		cookies, err := d.store.LoadCookies(ctx, host)
		if err == nil && len(cookies) > 0 {
			jar := tlsclient.NewCookieJar()
			fCookies := make([]*http.Cookie, len(cookies))
			for i, c := range cookies {
				fCookies[i] = &http.Cookie{
					Name:     c.Name,
					Value:    c.Value,
					Path:     c.Path,
					Domain:   c.Domain,
					Expires:  c.Expires,
					Secure:   c.Secure,
					HttpOnly: c.HttpOnly,
				}
			}
			base := &url.URL{Scheme: "https", Host: host, Path: "/"}
			jar.SetCookies(base, fCookies)
			options = append(options, tlsclient.WithCookieJar(jar))
		}
	}

	return tlsclient.NewHttpClient(tlsclient.NewNoopLogger(), options...)
}

func tlsProfileFor(id string) (profiles.ClientProfile, error) {
	switch id {
	case "chrome_124":
		return profiles.Chrome_124, nil
	default:
		return profiles.ClientProfile{}, fmt.Errorf("retrieval: unknown TLS profile %q", id)
	}
}

func headerExists(headers map[string]string, key string) bool {
	if headers == nil {
		return false
	}
	_, ok := headers[key]
	return ok
}
