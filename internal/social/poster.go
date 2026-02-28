package social

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/MikeS071/contentai/internal/config"
)

// SocialPoster sends one post to a social platform.
type SocialPoster interface {
	Post(ctx context.Context, post SocialPost) (PostResult, error)
}

// SocialPosterFunc adapts a function into a SocialPoster.
type SocialPosterFunc func(ctx context.Context, post SocialPost) (PostResult, error)

func (f SocialPosterFunc) Post(ctx context.Context, post SocialPost) (PostResult, error) {
	return f(ctx, post)
}

type SocialPost struct {
	Text       string
	ImageURL   string
	ImagePath  string
	ArticleURL string
	Platform   string // "x" or "linkedin"
}

type PostResult struct {
	Platform string
	ID       string
	URL      string
}

func NewPosterForPlatform(cfg *config.Config, platform string) (SocialPoster, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}
	name := strings.ToLower(strings.TrimSpace(platform))
	platformCfg, ok := cfg.Social[name]
	if !ok {
		return nil, fmt.Errorf("social.%s config missing", name)
	}
	if !platformCfg.Enabled {
		return nil, fmt.Errorf("social.%s is disabled", name)
	}

	switch name {
	case "x":
		key, err := config.ExecuteKeyCmd(platformCfg.APIKeyCmd)
		if err != nil {
			return nil, fmt.Errorf("resolve x api key: %w", err)
		}
		secret, err := config.ExecuteKeyCmd(platformCfg.APISecretCmd)
		if err != nil {
			return nil, fmt.Errorf("resolve x api secret: %w", err)
		}
		accessToken, err := config.ExecuteKeyCmd(platformCfg.AccessTokenCmd)
		if err != nil {
			return nil, fmt.Errorf("resolve x access token: %w", err)
		}
		accessSecret, err := config.ExecuteKeyCmd(platformCfg.AccessSecretCmd)
		if err != nil {
			return nil, fmt.Errorf("resolve x access secret: %w", err)
		}
		return NewXPoster(XPosterConfig{
			APIKey:       key,
			APISecret:    secret,
			AccessToken:  accessToken,
			AccessSecret: accessSecret,
		}), nil
	case "linkedin":
		accessToken, err := config.ExecuteKeyCmd(platformCfg.APIKeyCmd)
		if err != nil {
			return nil, fmt.Errorf("resolve linkedin access token: %w", err)
		}
		return NewLinkedInPoster(LinkedInPosterConfig{
			AccessToken: accessToken,
			AuthorURN:   strings.TrimSpace(platformCfg.AuthorURN),
		}), nil
	default:
		return nil, fmt.Errorf("unsupported social platform %q", platform)
	}
}
