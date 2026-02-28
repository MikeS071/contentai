package cmd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/social"
	"github.com/spf13/cobra"
)

func newPostCmd() *cobra.Command {
	var (
		confirm bool
		check   bool
	)

	cmd := &cobra.Command{
		Use:   "post [slug]",
		Short: "Post social copy now (requires --confirm) or run due approved scheduled posts",
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 1 {
				return fmt.Errorf("accepts at most one slug")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if check {
				if len(args) != 0 {
					return fmt.Errorf("post --check does not accept a slug")
				}
				return runPostCheck(cmd.Context())
			}
			if len(args) != 1 {
				return fmt.Errorf("slug is required unless using --check")
			}
			if !confirm {
				return fmt.Errorf("refusing to post without --confirm")
			}
			return runPostSlug(cmd.Context(), args[0])
		},
	}

	cmd.Flags().BoolVar(&confirm, "confirm", false, "Required safety gate for any manual post")
	cmd.Flags().BoolVar(&check, "check", false, "Post due and approved scheduled slots")
	return cmd
}

func runPostSlug(ctx context.Context, slug string) error {
	cfg, err := loadDraftConfig(cfgFile)
	if err != nil {
		return err
	}
	contentDir := strings.TrimSpace(cfg.Project.ContentDir)
	if contentDir == "" {
		contentDir = "content"
	}

	store := content.NewStore(contentDir)
	scheduler := social.NewScheduler(filepath.Join(contentDir, "posting-calendar.json"), cfg.Schedule)
	now := time.Now().UTC()
	if err := approvePendingSlots(scheduler, slug, now); err != nil {
		return err
	}

	svc := &social.PostingService{
		Store:     store,
		Scheduler: scheduler,
		Now:       time.Now,
		PosterForPlatform: func(_ context.Context, platform string) (social.SocialPoster, error) {
			return social.NewPosterForPlatform(cfg, platform)
		},
	}
	_, err = svc.PostSlug(ctx, slug, true)
	return err
}

func runPostCheck(ctx context.Context) error {
	cfg, err := loadDraftConfig(cfgFile)
	if err != nil {
		return err
	}
	contentDir := strings.TrimSpace(cfg.Project.ContentDir)
	if contentDir == "" {
		contentDir = "content"
	}

	svc := &social.PostingService{
		Store:     content.NewStore(contentDir),
		Scheduler: social.NewScheduler(filepath.Join(contentDir, "posting-calendar.json"), cfg.Schedule),
		Now:       time.Now,
		PosterForPlatform: func(_ context.Context, platform string) (social.SocialPoster, error) {
			return social.NewPosterForPlatform(cfg, platform)
		},
	}
	_, err = svc.PostDue(ctx)
	return err
}

func approvePendingSlots(s *social.Scheduler, slug string, now time.Time) error {
	cal, err := s.Load()
	if err != nil {
		return err
	}
	changed := false
	for i := range cal.Slots {
		slot := &cal.Slots[i]
		if slot.Slug != slug || slot.Status == social.SlotStatusPosted {
			continue
		}
		slot.Approved = true
		slot.UpdatedAt = now.UTC()
		changed = true
	}
	if changed {
		return s.Save(cal)
	}
	return nil
}
