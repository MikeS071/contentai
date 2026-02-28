package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/MikeS071/contentai/internal/config"
	"github.com/MikeS071/contentai/internal/kb"
	"github.com/spf13/cobra"
)

func newKBCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "kb",
		Short: "Manage knowledge base feeds, notes, and search",
	}

	cmd.AddCommand(newKBAddFeedCmd())
	cmd.AddCommand(newKBListFeedsCmd())
	cmd.AddCommand(newKBSyncCmd())
	cmd.AddCommand(newKBAddNoteCmd())
	cmd.AddCommand(newKBSearchCmd())

	return cmd
}

func newKBAddFeedCmd() *cobra.Command {
	var opmlPath string
	cmd := &cobra.Command{
		Use:   "add-feed [url]",
		Short: "Add RSS/Atom feed or import OPML",
		Args: func(_ *cobra.Command, args []string) error {
			if strings.TrimSpace(opmlPath) == "" && len(args) != 1 {
				return fmt.Errorf("requires exactly 1 feed URL argument unless --opml is set")
			}
			if strings.TrimSpace(opmlPath) != "" && len(args) > 1 {
				return fmt.Errorf("accepts at most 1 URL argument with --opml")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newKBStoreFromConfig(cfgFile)
			if err != nil {
				return err
			}
			if strings.TrimSpace(opmlPath) != "" {
				count, err := store.ImportOPML(opmlPath)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "imported %d feeds\n", count)
				return nil
			}

			feed, err := store.AddFeed(args[0], "")
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added feed: %s\n", feed.URL)
			return nil
		},
	}
	cmd.Flags().StringVar(&opmlPath, "opml", "", "Import feeds from OPML file")
	return cmd
}

func newKBListFeedsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list-feeds",
		Short: "List tracked feeds",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := newKBStoreFromConfig(cfgFile)
			if err != nil {
				return err
			}
			feeds, err := store.ListFeeds()
			if err != nil {
				return err
			}
			for _, feed := range feeds {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\n", feed.Title, feed.URL)
			}
			return nil
		},
	}
}

func newKBSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Fetch latest posts from all tracked feeds",
		RunE: func(cmd *cobra.Command, _ []string) error {
			store, err := newKBStoreFromConfig(cfgFile)
			if err != nil {
				return err
			}
			report, err := store.Sync(context.Background())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "feeds=%d synced=%d new_posts=%d\n", report.Feeds, report.SyncedFeeds, report.NewPosts)
			for _, e := range report.Errors {
				fmt.Fprintln(cmd.ErrOrStderr(), e)
			}
			return nil
		},
	}
}

func newKBAddNoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add-note <path>",
		Short: "Add note/transcript markdown into KB",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newKBStoreFromConfig(cfgFile)
			if err != nil {
				return err
			}
			dst, err := store.AddNote(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added note: %s\n", dst)
			return nil
		},
	}
}

func newKBSearchCmd() *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search keyword across KB content",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store, err := newKBStoreFromConfig(cfgFile)
			if err != nil {
				return err
			}
			results, err := kb.Search(filepath.Join(store.ContentDir, "kb"), args[0], limit)
			if err != nil {
				return err
			}
			for _, r := range results {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n%s\n%s\n\n", r.Title, r.Snippet, r.SourcePath)
			}
			return nil
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 10, "Maximum number of results")
	return cmd
}

func newKBStoreFromConfig(configPath string) (*kb.Store, error) {
	contentDir := "content"
	if configPath != "" {
		if _, err := os.Stat(configPath); err == nil {
			cfg, loadErr := config.Load(configPath)
			if loadErr != nil {
				return nil, loadErr
			}
			if strings.TrimSpace(cfg.Project.ContentDir) != "" {
				contentDir = cfg.Project.ContentDir
			}
		}
	}
	return kb.NewStore(contentDir), nil
}
