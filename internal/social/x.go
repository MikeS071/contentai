package social

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	defaultXAPIBaseURL = "https://api.x.com"
	defaultXUploadURL  = "https://upload.twitter.com"
)

type XPosterConfig struct {
	APIKey       string
	APISecret    string
	AccessToken  string
	AccessSecret string
	APIBaseURL   string
	UploadURL    string
	HTTPClient   *http.Client
	Now          func() time.Time
}

type XPoster struct {
	apiKey       string
	apiSecret    string
	accessToken  string
	accessSecret string
	apiBaseURL   string
	uploadURL    string
	httpClient   *http.Client
	now          func() time.Time
}

func NewXPoster(cfg XPosterConfig) *XPoster {
	apiBase := strings.TrimSpace(cfg.APIBaseURL)
	if apiBase == "" {
		apiBase = defaultXAPIBaseURL
	}
	uploadBase := strings.TrimSpace(cfg.UploadURL)
	if uploadBase == "" {
		uploadBase = defaultXUploadURL
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}
	return &XPoster{
		apiKey:       strings.TrimSpace(cfg.APIKey),
		apiSecret:    strings.TrimSpace(cfg.APISecret),
		accessToken:  strings.TrimSpace(cfg.AccessToken),
		accessSecret: strings.TrimSpace(cfg.AccessSecret),
		apiBaseURL:   strings.TrimRight(apiBase, "/"),
		uploadURL:    strings.TrimRight(uploadBase, "/"),
		httpClient:   client,
		now:          nowFn,
	}
}

func (p *XPoster) Post(ctx context.Context, post SocialPost) (PostResult, error) {
	if strings.TrimSpace(post.Text) == "" {
		return PostResult{}, fmt.Errorf("x post text is required")
	}

	var mediaID string
	if strings.TrimSpace(post.ImagePath) != "" {
		id, err := p.uploadMedia(ctx, strings.TrimSpace(post.ImagePath))
		if err != nil {
			return PostResult{}, err
		}
		mediaID = id
	}

	payload := map[string]any{"text": post.Text}
	if mediaID != "" {
		payload["media"] = map[string]any{"media_ids": []string{mediaID}}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return PostResult{}, fmt.Errorf("encode x tweet payload: %w", err)
	}

	tweetsURL := p.apiBaseURL + "/2/tweets"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tweetsURL, bytes.NewReader(body))
	if err != nil {
		return PostResult{}, fmt.Errorf("create x tweet request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", p.oauthHeader(http.MethodPost, tweetsURL))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return PostResult{}, fmt.Errorf("send x tweet request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return PostResult{}, fmt.Errorf("x tweet request failed: status %d body %q", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var out struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return PostResult{}, fmt.Errorf("decode x tweet response: %w", err)
	}

	result := PostResult{Platform: "x", ID: out.Data.ID}
	if out.Data.ID != "" {
		result.URL = "https://x.com/i/web/status/" + out.Data.ID
	}
	return result, nil
}

func (p *XPoster) uploadMedia(ctx context.Context, imagePath string) (string, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return "", fmt.Errorf("open x image %q: %w", imagePath, err)
	}
	defer f.Close()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("media", filepath.Base(imagePath))
	if err != nil {
		return "", fmt.Errorf("create x media form file: %w", err)
	}
	if _, err := io.Copy(part, f); err != nil {
		return "", fmt.Errorf("copy x media form file: %w", err)
	}
	if err := mw.Close(); err != nil {
		return "", fmt.Errorf("close x media multipart: %w", err)
	}

	uploadURL := p.uploadURL + "/1.1/media/upload.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, &body)
	if err != nil {
		return "", fmt.Errorf("create x media upload request: %w", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", p.oauthHeader(http.MethodPost, uploadURL))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send x media upload request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("x media upload failed: status %d body %q", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var out struct {
		MediaIDString string `json:"media_id_string"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decode x media upload response: %w", err)
	}
	if strings.TrimSpace(out.MediaIDString) == "" {
		return "", fmt.Errorf("x media upload did not return media_id_string")
	}
	return out.MediaIDString, nil
}

func (p *XPoster) oauthHeader(method, rawURL string) string {
	timestamp := strconv.FormatInt(p.now().UTC().Unix(), 10)
	nonce := strconv.FormatInt(p.now().UTC().UnixNano(), 10)
	params := []oauthParam{
		{"oauth_consumer_key", p.apiKey},
		{"oauth_token", p.accessToken},
		{"oauth_nonce", nonce},
		{"oauth_signature_method", "HMAC-SHA1"},
		{"oauth_timestamp", timestamp},
		{"oauth_version", "1.0"},
	}
	signature := oauthSignature(method, rawURL, params, p.apiSecret, p.accessSecret)
	params = append(params, oauthParam{"oauth_signature", signature})

	parts := make([]string, 0, len(params))
	for _, p := range params {
		parts = append(parts, fmt.Sprintf("%s=\"%s\"", oauthEscape(p.k), oauthEscape(p.v)))
	}
	return "OAuth " + strings.Join(parts, ", ")
}

type oauthParam struct {
	k string
	v string
}

func oauthSignature(method, rawURL string, params []oauthParam, consumerSecret, accessSecret string) string {
	pairs := make([]string, 0, len(params))
	for _, p := range params {
		pairs = append(pairs, oauthEscape(p.k)+"="+oauthEscape(p.v))
	}
	sortStrings(pairs)
	normalizedParams := strings.Join(pairs, "&")
	base := strings.ToUpper(method) + "&" + oauthEscape(stripQuery(rawURL)) + "&" + oauthEscape(normalizedParams)
	key := oauthEscape(consumerSecret) + "&" + oauthEscape(accessSecret)

	mac := hmac.New(sha1.New, []byte(key))
	_, _ = mac.Write([]byte(base))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func oauthEscape(v string) string {
	return strings.ReplaceAll(url.QueryEscape(v), "+", "%20")
}

func stripQuery(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

func sortStrings(values []string) {
	for i := 0; i < len(values); i++ {
		for j := i + 1; j < len(values); j++ {
			if values[j] < values[i] {
				values[i], values[j] = values[j], values[i]
			}
		}
	}
}
