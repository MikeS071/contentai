package cmd

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/MikeS071/contentai/internal/content"
)

func RunList(args []string, stdout io.Writer, stderr io.Writer, store *content.Store) error {
	fs := flag.NewFlagSet("list", flag.ContinueOnError)
	fs.SetOutput(stderr)

	statusArg := fs.String("status", "", "filter by status")
	jsonOut := fs.Bool("json", false, "emit JSON")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("usage: contentai list [--status STATUS] [--json]")
	}

	var filter *content.Status
	if *statusArg != "" {
		status, err := content.ParseStatus(*statusArg)
		if err != nil {
			return err
		}
		filter = &status
	}

	items, err := store.List(filter)
	if err != nil {
		return err
	}

	if *jsonOut {
		enc := json.NewEncoder(stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}

	tw := tabwriter.NewWriter(stdout, 0, 8, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "SLUG\tSTATUS\tTITLE\tCREATED"); err != nil {
		return err
	}
	for _, item := range items {
		created := item.CreatedAt.Format(time.RFC3339)
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", item.Slug, item.Status, item.Title, created); err != nil {
			return err
		}
	}
	return tw.Flush()
}
