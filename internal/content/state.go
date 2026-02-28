package content

import (
	"fmt"
	"time"
)

func ValidTransition(from, to Status, qaGate bool) error {
	if !IsValidStatus(from) {
		return fmt.Errorf("invalid transition: unknown from status %q", from)
	}
	if !IsValidStatus(to) {
		return fmt.Errorf("invalid transition: unknown to status %q", to)
	}
	if from == to {
		return fmt.Errorf("invalid transition: %q -> %q", from, to)
	}

	valid := map[Status]map[Status]bool{
		StatusDraft: {
			StatusQAPassed: true,
		},
		StatusQAPassed: {
			StatusDraft:     true,
			StatusPublished: true,
		},
		StatusPublished: {
			StatusSocialGenerated: true,
		},
		StatusSocialGenerated: {
			StatusScheduled: true,
		},
		StatusScheduled: {
			StatusPosted: true,
		},
	}

	if !qaGate && from == StatusDraft && to == StatusPublished {
		return nil
	}
	if next, ok := valid[from]; ok && next[to] {
		return nil
	}
	return fmt.Errorf("invalid transition: %q -> %q", from, to)
}

func (s *Store) Transition(slug string, to Status, qaGate bool) error {
	meta, err := s.Get(slug)
	if err != nil {
		return err
	}
	if err := ValidTransition(meta.Status, to, qaGate); err != nil {
		return err
	}
	meta.Status = to
	meta.UpdatedAt = time.Now().UTC()
	if to == StatusPublished && meta.PublishedAt == nil {
		publishedAt := meta.UpdatedAt
		meta.PublishedAt = &publishedAt
	}
	return s.UpdateMeta(slug, meta)
}
