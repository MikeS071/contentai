package cmd

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/MikeS071/contentai/internal/content"
	"github.com/MikeS071/contentai/internal/social"
	"github.com/spf13/cobra"
)

func newScheduleCmd() *cobra.Command {
	var slotArg string

	cmd := &cobra.Command{
		Use:   "schedule <slug>",
		Short: "Schedule a social post slot for a slug",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadDraftConfig(cfgFile)
			if err != nil {
				return err
			}
			contentDir := strings.TrimSpace(cfg.Project.ContentDir)
			if contentDir == "" {
				contentDir = "content"
			}
			slug := strings.TrimSpace(args[0])
			if err := content.ValidateSlug(slug); err != nil {
				return err
			}

			store := content.NewStore(contentDir)
			meta, err := store.Get(slug)
			if err != nil {
				return err
			}

			scheduler := social.NewScheduler(filepath.Join(contentDir, "posting-calendar.json"), cfg.Schedule)
			now := time.Now().UTC()
			slotTime, err := resolveScheduleSlot(scheduler, now, slotArg)
			if err != nil {
				return err
			}

			if err := scheduler.Add(slug, slotTime); err != nil {
				return err
			}
			if meta.Status == content.StatusSocialGenerated {
				if err := store.Transition(slug, content.StatusScheduled, cfg.Project.QAGate); err != nil {
					return err
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Scheduled %s at %s (pending explicit approval)\n", slug, slotTime.Format(time.RFC3339))
			return nil
		},
	}
	cmd.Flags().StringVar(&slotArg, "slot", "next", "Schedule slot: next or YYYY-MM-DD")
	return cmd
}

func resolveScheduleSlot(s *social.Scheduler, now time.Time, slotArg string) (time.Time, error) {
	arg := strings.TrimSpace(slotArg)
	if arg == "" || strings.EqualFold(arg, "next") {
		return s.NextSlot(now)
	}
	day, err := time.Parse("2006-01-02", arg)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --slot %q: use next or YYYY-MM-DD", arg)
	}
	candidate := day.UTC()
	next, err := s.NextSlot(candidate)
	if err != nil {
		return time.Time{}, err
	}
	// Keep exact day if it is valid; otherwise return the scheduler-calculated next day.
	if next.Year() == candidate.Year() && next.YearDay() == candidate.YearDay() {
		return next, nil
	}
	return next, nil
}
