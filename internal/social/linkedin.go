package social

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

const defaultLinkedInBaseURL = "https://api.linkedin.com"

type LinkedInPosterConfig struct {
	AccessToken string
	AuthorURN   string
	BaseURL     string
	HTTPClient  *http.Client
}

type LinkedInPoster struct {
	accessToken string
	authorURN   string
	baseURL     string
	httpClient  *http.Client
}

func NewLinkedInPoster(cfg LinkedInPosterConfig) *LinkedInPoster {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultLinkedInBaseURL
	}
	client := cfg.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	return &LinkedInPoster{
		accessToken: strings.TrimSpace(cfg.AccessToken),
		authorURN:   strings.TrimSpace(cfg.AuthorURN),
		baseURL:     strings.TrimRight(baseURL, "/"),
		httpClient:  client,
	}
}

func (p *LinkedInPoster) Post(ctx context.Context, post SocialPost) (PostResult, error) {
	if strings.TrimSpace(post.Text) == "" {
		return PostResult{}, fmt.Errorf("linkedin post text is required")
	}
	if p.authorURN == "" {
		return PostResult{}, fmt.Errorf("linkedin author urn is required")
	}

	assetURN := ""
	if strings.TrimSpace(post.ImagePath) != "" {
		asset, err := p.uploadImage(ctx, post.ImagePath)
		if err != nil {
			return PostResult{}, err
		}
		assetURN = asset
	}

	shareContent := map[string]any{
		"shareCommentary":    map[string]string{"text": post.Text},
		"shareMediaCategory": "NONE",
		"media":              []map[string]any{},
	}
	if assetURN != "" {
		shareContent["shareMediaCategory"] = "IMAGE"
		shareContent["media"] = []map[string]any{{"status": "READY", "media": assetURN}}
	} else if strings.TrimSpace(post.ImageURL) != "" {
		shareContent["shareMediaCategory"] = "IMAGE"
		shareContent["media"] = []map[string]any{{"status": "READY", "originalUrl": strings.TrimSpace(post.ImageURL)}}
	}

	payload := map[string]any{
		"author":         p.authorURN,
		"lifecycleState": "PUBLISHED",
		"specificContent": map[string]any{
			"com.linkedin.ugc.ShareContent": shareContent,
		},
		"visibility": map[string]string{"com.linkedin.ugc.MemberNetworkVisibility": "PUBLIC"},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return PostResult{}, fmt.Errorf("encode linkedin ugc payload: %w", err)
	}

	ugcURL := p.baseURL + "/v2/ugcPosts"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ugcURL, bytes.NewReader(body))
	if err != nil {
		return PostResult{}, fmt.Errorf("create linkedin post request: %w", err)
	}
	p.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return PostResult{}, fmt.Errorf("send linkedin post request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return PostResult{}, fmt.Errorf("linkedin post failed: status %d body %q", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var out struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return PostResult{Platform: "linkedin", ID: out.ID, URL: out.ID}, nil
}

func (p *LinkedInPoster) uploadImage(ctx context.Context, imagePath string) (string, error) {
	registerPayload := map[string]any{
		"registerUploadRequest": map[string]any{
			"recipes":              []string{"urn:li:digitalmediaRecipe:feedshare-image"},
			"owner":                p.authorURN,
			"serviceRelationships": []map[string]string{{"relationshipType": "OWNER", "identifier": "urn:li:userGeneratedContent"}},
		},
	}
	body, err := json.Marshal(registerPayload)
	if err != nil {
		return "", fmt.Errorf("encode linkedin register upload payload: %w", err)
	}

	registerURL := p.baseURL + "/v2/assets?action=registerUpload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, registerURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create linkedin register upload request: %w", err)
	}
	p.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("send linkedin register upload request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("linkedin register upload failed: status %d body %q", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	var reg struct {
		Value struct {
			UploadMechanism struct {
				MediaUploadHTTPReq struct {
					UploadURL string `json:"uploadUrl"`
				} `json:"com.linkedin.digitalmedia.uploading.MediaUploadHttpRequest"`
			} `json:"uploadMechanism"`
			Asset string `json:"asset"`
		} `json:"value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return "", fmt.Errorf("decode linkedin register upload response: %w", err)
	}
	uploadURL := strings.TrimSpace(reg.Value.UploadMechanism.MediaUploadHTTPReq.UploadURL)
	asset := strings.TrimSpace(reg.Value.Asset)
	if uploadURL == "" || asset == "" {
		return "", fmt.Errorf("linkedin register upload returned incomplete payload")
	}

	fileData, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("read linkedin image %q: %w", imagePath, err)
	}

	putReq, err := http.NewRequestWithContext(ctx, http.MethodPut, uploadURL, bytes.NewReader(fileData))
	if err != nil {
		return "", fmt.Errorf("create linkedin image upload request: %w", err)
	}
	putReq.Header.Set("Authorization", "Bearer "+p.accessToken)
	putReq.Header.Set("Content-Type", detectContentType(imagePath))

	putResp, err := p.httpClient.Do(putReq)
	if err != nil {
		return "", fmt.Errorf("send linkedin image upload request: %w", err)
	}
	defer putResp.Body.Close()
	if putResp.StatusCode < 200 || putResp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(putResp.Body, 4096))
		return "", fmt.Errorf("linkedin image upload failed: status %d body %q", putResp.StatusCode, strings.TrimSpace(string(data)))
	}

	return asset, nil
}

func (p *LinkedInPoster) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+p.accessToken)
	req.Header.Set("X-Restli-Protocol-Version", "2.0.0")
}

func detectContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		return "application/octet-stream"
	}
	if ct := mime.TypeByExtension(ext); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
