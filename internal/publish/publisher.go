package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/MikeS071/contentai/internal/config"
	"github.com/MikeS071/contentai/internal/content"
)

type Publisher interface {
	Publish(ctx context.Context, item PublishItem) (PublishResult, error)
}

type PublishItem struct {
	Title     string
	Slug      string
	Content   string
	Summary   string
	ImageURL  string
	ImagePath string
	Meta      map[string]any
}

type PublishResult struct {
	URL string
	ID  string
}

type ServiceConfig struct {
	RequireApprove bool
	QAGate         bool
}

type PublishOptions struct {
	Approve bool
	DryRun  bool
}

type PublishOutput struct {
	DryRun      bool
	PayloadJSON []byte
	Result      PublishResult
}

type Service struct {
	store     *content.Store
	publisher Publisher
	cfg       ServiceConfig
}

func NewService(store *content.Store, publisher Publisher, cfg ServiceConfig) *Service {
	return &Service{store: store, publisher: publisher, cfg: cfg}
}

func NewPublisherFromConfig(cfg config.PublishConfig) (Publisher, error) {
	switch strings.TrimSpace(cfg.Type) {
	case "http", "":
		authToken := ""
		cmd := strings.TrimSpace(cfg.APIKeyCmd)
		if cmd == "" {
			cmd = strings.TrimSpace(cfg.AuthCmd)
		}
		if cmd != "" {
			resolved, err := config.ExecuteKeyCmd(cmd)
			if err != nil {
				return nil, fmt.Errorf("resolve publish auth token: %w", err)
			}
			authToken = resolved
		}

		return NewHTTPPublisher(HTTPConfig{
			URL:             strings.TrimSpace(cfg.URL),
			FieldMap:        cfg.FieldMap,
			AuthHeader:      strings.TrimSpace(cfg.AuthHeader),
			AuthToken:       authToken,
			AuthPrefix:      cfg.AuthPrefix,
			ResponseURLPath: strings.TrimSpace(cfg.ResponseURLPath),
		}), nil
	case "static":
		return NewStaticPublisher(StaticConfig{OutputDir: strings.TrimSpace(cfg.Static.OutputDir)}), nil
	default:
		return nil, fmt.Errorf("unsupported publish.type %q", cfg.Type)
	}
}

func (s *Service) PublishSlug(ctx context.Context, slug string, opts PublishOptions) (PublishOutput, error) {
	if s == nil || s.store == nil {
		return PublishOutput{}, errors.New("publish service is not initialized")
	}
	if strings.TrimSpace(slug) == "" {
		return PublishOutput{}, errors.New("slug is required")
	}

	meta, err := s.store.Get(slug)
	if err != nil {
		return PublishOutput{}, err
	}
	article, err := s.store.ReadArticle(slug)
	if err != nil {
		return PublishOutput{}, err
	}

	item := PublishItem{
		Title:   meta.Title,
		Slug:    meta.Slug,
		Content: article,
		Summary: meta.Summary,
		Meta: map[string]any{
			"status": string(meta.Status),
		},
	}
	if _, err := os.Stat(s.store.HeroPath(slug)); err == nil {
		item.ImagePath = s.store.HeroPath(slug)
	}

	if s.cfg.QAGate && meta.Status != content.StatusQAPassed {
		return PublishOutput{}, fmt.Errorf("cannot publish %q: status must be %q", slug, content.StatusQAPassed)
	}
	if err := content.ValidTransition(meta.Status, content.StatusPublished, s.cfg.QAGate); err != nil {
		return PublishOutput{}, err
	}

	payloadJSON, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return PublishOutput{}, fmt.Errorf("marshal publish payload: %w", err)
	}
	payloadJSON = append(payloadJSON, '\n')

	if opts.DryRun || (s.cfg.RequireApprove && !opts.Approve) {
		return PublishOutput{DryRun: true, PayloadJSON: payloadJSON}, nil
	}
	if s.publisher == nil {
		return PublishOutput{}, errors.New("publisher is not configured")
	}

	result, err := s.publisher.Publish(ctx, item)
	if err != nil {
		return PublishOutput{}, err
	}

	now := time.Now().UTC()
	meta.Status = content.StatusPublished
	meta.UpdatedAt = now
	if meta.PublishedAt == nil {
		meta.PublishedAt = &now
	}
	meta.PublishURL = result.URL
	if err := s.store.UpdateMeta(slug, meta); err != nil {
		return PublishOutput{}, err
	}

	return PublishOutput{Result: result}, nil
}
