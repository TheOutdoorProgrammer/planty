package nativesymbols

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

func Publish(ctx context.Context, baseURL, tokenURL, requestToken, imageUUID, architecture string, data []byte) error {
	client := &http.Client{
		Timeout:       90 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return publish(ctx, client, baseURL, tokenURL, requestToken, imageUUID, architecture, data)
}

func publish(ctx context.Context, client *http.Client, baseURL, tokenURL, requestToken, imageUUID, architecture string, data []byte) error {
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme != "https" || base.Host == "" || base.User != nil || base.RawQuery != "" || base.Fragment != "" || (base.Path != "" && base.Path != "/") {
		return errors.New("symbol API requires an HTTPS origin")
	}
	issuer, err := url.Parse(tokenURL)
	if err != nil || issuer.Scheme != "https" || issuer.Host == "" || issuer.User != nil || issuer.Fragment != "" || requestToken == "" {
		return errors.New("GitHub workload identity is unavailable")
	}
	if len(data) == 0 || len(data) > MaxSymbolBytes {
		return errors.New("symbol object size is invalid")
	}
	query := issuer.Query()
	query.Set("audience", Audience)
	issuer.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, issuer.String(), nil)
	if err != nil {
		return errors.New("cannot create workload identity request")
	}
	req.Header.Set("Authorization", "Bearer "+requestToken)
	response, err := client.Do(req)
	if err != nil {
		return errors.New("workload identity request failed")
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 65537))
	_ = response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || len(body) > 65536 {
		return errors.New("workload identity request was not accepted")
	}
	var identity struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(body, &identity) != nil || identity.Value == "" || len(identity.Value) > 16384 {
		return errors.New("workload identity response is invalid")
	}
	endpoint := base.JoinPath("v1", "native-symbols", imageUUID, architecture)
	req, err = http.NewRequestWithContext(ctx, http.MethodPut, endpoint.String(), bytes.NewReader(data))
	if err != nil {
		return errors.New("cannot create symbol upload")
	}
	req.Header.Set("Authorization", "Bearer "+identity.Value)
	req.Header.Set("Content-Type", "application/octet-stream")
	response, err = client.Do(req)
	if err != nil {
		return errors.New("symbol upload failed")
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusNoContent {
		return fmt.Errorf("symbol upload returned HTTP %d", response.StatusCode)
	}
	return nil
}
