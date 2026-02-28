package content

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	metaFileName    = "meta.json"
	articleFileName = "article.md"
	socialFileName  = "social.json"
	qaFileName      = "qa.json"
)

type Store struct {
	ContentDir string
}

func NewStore(contentDir string) *Store {
	return &Store{ContentDir: contentDir}
}

func (s *Store) Create(slug, title string) error {
	if err := ValidateSlug(slug); err != nil {
		return err
	}

	dir := s.SlugDir(slug)
	if s.Exists(slug) {
		return fmt.Errorf("slug %q already exists", slug)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create slug dir: %w", err)
	}

	now := time.Now().UTC()
	meta := &Meta{
		Title:     strings.TrimSpace(title),
		Slug:      slug,
		Status:    StatusDraft,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.writeJSON(filepath.Join(dir, metaFileName), meta); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, articleFileName), nil, 0o644); err != nil {
		return fmt.Errorf("write article.md: %w", err)
	}
	if err := s.writeJSON(filepath.Join(dir, socialFileName), &SocialJSON{}); err != nil {
		return err
	}
	return nil
}

func (s *Store) Get(slug string) (*Meta, error) {
	var meta Meta
	if err := s.readJSON(filepath.Join(s.SlugDir(slug), metaFileName), &meta); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &meta, nil
}

func (s *Store) List(statusFilter *Status) ([]Meta, error) {
	entries, err := os.ReadDir(s.ContentDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read content dir: %w", err)
	}

	items := make([]Meta, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		slug := entry.Name()
		if err := ValidateSlug(slug); err != nil {
			continue
		}

		meta, err := s.Get(slug)
		if err != nil {
			if errors.Is(err, ErrNotFound) {
				continue
			}
			return nil, err
		}
		if statusFilter != nil && meta.Status != *statusFilter {
			continue
		}
		items = append(items, *meta)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAt.After(items[j].CreatedAt)
	})

	return items, nil
}

func (s *Store) UpdateMeta(slug string, meta *Meta) error {
	if err := ValidateSlug(slug); err != nil {
		return err
	}
	if meta == nil {
		return errors.New("meta is nil")
	}
	if meta.Slug != slug {
		return fmt.Errorf("meta slug mismatch: %q != %q", meta.Slug, slug)
	}
	if !IsValidStatus(meta.Status) {
		return fmt.Errorf("invalid status %q", meta.Status)
	}
	return s.writeJSON(filepath.Join(s.SlugDir(slug), metaFileName), meta)
}

func (s *Store) ReadArticle(slug string) (string, error) {
	content, err := os.ReadFile(filepath.Join(s.SlugDir(slug), articleFileName))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", ErrNotFound
		}
		return "", fmt.Errorf("read article.md: %w", err)
	}
	return string(content), nil
}

func (s *Store) WriteArticle(slug, content string) error {
	if err := os.WriteFile(filepath.Join(s.SlugDir(slug), articleFileName), []byte(content), 0o644); err != nil {
		return fmt.Errorf("write article.md: %w", err)
	}
	return nil
}

func (s *Store) ReadSocial(slug string) (*SocialJSON, error) {
	var social SocialJSON
	if err := s.readJSON(filepath.Join(s.SlugDir(slug), socialFileName), &social); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &social, nil
}

func (s *Store) WriteSocial(slug string, social *SocialJSON) error {
	if social == nil {
		return errors.New("social is nil")
	}
	return s.writeJSON(filepath.Join(s.SlugDir(slug), socialFileName), social)
}

func (s *Store) ReadQA(slug string) (*QAJSON, error) {
	var qa QAJSON
	if err := s.readJSON(filepath.Join(s.SlugDir(slug), qaFileName), &qa); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	return &qa, nil
}

func (s *Store) WriteQA(slug string, qa *QAJSON) error {
	if qa == nil {
		return errors.New("qa is nil")
	}
	return s.writeJSON(filepath.Join(s.SlugDir(slug), qaFileName), qa)
}

func (s *Store) Exists(slug string) bool {
	info, err := os.Stat(s.SlugDir(slug))
	return err == nil && info.IsDir()
}

func (s *Store) HeroPath(slug string) string {
	return filepath.Join(s.SlugDir(slug), "hero.png")
}

func (s *Store) HeroLinkedInPath(slug string) string {
	return filepath.Join(s.SlugDir(slug), "hero-linkedin.png")
}

func (s *Store) SlugDir(slug string) string {
	return filepath.Join(s.ContentDir, slug)
}

func (s *Store) readJSON(path string, target any) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(content, target); err != nil {
		return fmt.Errorf("decode %s: %w", filepath.Base(path), err)
	}
	return nil
}

func (s *Store) writeJSON(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create parent dir for %s: %w", filepath.Base(path), err)
	}
	content, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}
