package social

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/MikeS071/contentai/internal/config"
	"github.com/MikeS071/contentai/internal/content"
)

type SlotStatus string

const (
	SlotStatusScheduled SlotStatus = "scheduled"
	SlotStatusPosted    SlotStatus = "posted"
	SlotStatusFailed    SlotStatus = "failed"
)

type PostingCalendar struct {
	Slots []CalendarSlot `json:"slots"`
}

type CalendarSlot struct {
	Slug      string     `json:"slug"`
	SlotTime  time.Time  `json:"slot_time"`
	Approved  bool       `json:"approved"`
	Status    SlotStatus `json:"status"`
	PostedAt  *time.Time `json:"posted_at,omitempty"`
	Error     string     `json:"error,omitempty"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type Scheduler struct {
	path    string
	sched   config.ScheduleConfig
	nowFunc func() time.Time
}

func NewScheduler(path string, sched config.ScheduleConfig) *Scheduler {
	return &Scheduler{
		path:    path,
		sched:   sched,
		nowFunc: time.Now,
	}
}

func (s *Scheduler) Load() (*PostingCalendar, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &PostingCalendar{Slots: []CalendarSlot{}}, nil
		}
		return nil, fmt.Errorf("read posting calendar: %w", err)
	}
	var cal PostingCalendar
	if err := json.Unmarshal(data, &cal); err != nil {
		return nil, fmt.Errorf("decode posting calendar: %w", err)
	}
	if cal.Slots == nil {
		cal.Slots = []CalendarSlot{}
	}
	return &cal, nil
}

func (s *Scheduler) Save(cal *PostingCalendar) error {
	if cal == nil {
		return errors.New("calendar is nil")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create posting calendar dir: %w", err)
	}
	content, err := json.MarshalIndent(cal, "", "  ")
	if err != nil {
		return fmt.Errorf("encode posting calendar: %w", err)
	}
	content = append(content, '\n')
	if err := os.WriteFile(s.path, content, 0o644); err != nil {
		return fmt.Errorf("write posting calendar: %w", err)
	}
	return nil
}

func (s *Scheduler) Add(slug string, slot time.Time) error {
	if err := content.ValidateSlug(slug); err != nil {
		return err
	}
	cal, err := s.Load()
	if err != nil {
		return err
	}
	now := s.nowFunc().UTC()
	cal.Slots = append(cal.Slots, CalendarSlot{
		Slug:      slug,
		SlotTime:  slot.UTC(),
		Approved:  false,
		Status:    SlotStatusScheduled,
		UpdatedAt: now,
	})
	sort.Slice(cal.Slots, func(i, j int) bool { return cal.Slots[i].SlotTime.Before(cal.Slots[j].SlotTime) })
	return s.Save(cal)
}

func (s *Scheduler) NextSlot(now time.Time) (time.Time, error) {
	loc, err := time.LoadLocation(strings.TrimSpace(s.sched.Timezone))
	if err != nil {
		loc = time.UTC
	}
	if len(s.sched.Days) == 0 {
		s.sched.Days = []string{"Mon", "Tue", "Wed", "Thu", "Fri"}
	}
	daySet := map[string]struct{}{}
	for _, d := range s.sched.Days {
		daySet[strings.TrimSpace(d)] = struct{}{}
	}
	startHM := strings.TrimSpace(s.sched.WindowStart)
	if startHM == "" {
		startHM = "09:00"
	}
	endHM := strings.TrimSpace(s.sched.WindowEnd)
	if endHM == "" {
		endHM = "09:30"
	}
	startHour, startMin, err := parseHourMinute(startHM)
	if err != nil {
		return time.Time{}, err
	}
	endHour, endMin, err := parseHourMinute(endHM)
	if err != nil {
		return time.Time{}, err
	}

	localNow := now.In(loc)
	for i := 0; i < 14; i++ {
		day := localNow.AddDate(0, 0, i)
		if _, ok := daySet[day.Weekday().String()[:3]]; !ok {
			continue
		}
		start := time.Date(day.Year(), day.Month(), day.Day(), startHour, startMin, 0, 0, loc)
		end := time.Date(day.Year(), day.Month(), day.Day(), endHour, endMin, 0, 0, loc)
		if !end.After(start) {
			return time.Time{}, fmt.Errorf("invalid schedule window %q-%q", startHM, endHM)
		}
		if i == 0 {
			if localNow.Before(start) || localNow.Equal(start) {
				return start.UTC(), nil
			}
			continue
		}
		return start.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("no available slot found")
}

func parseHourMinute(hm string) (int, int, error) {
	t, err := time.Parse("15:04", hm)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid HH:MM value %q", hm)
	}
	return t.Hour(), t.Minute(), nil
}

type PostingService struct {
	Store             *content.Store
	Scheduler         *Scheduler
	Now               func() time.Time
	PosterForPlatform func(ctx context.Context, platform string) (SocialPoster, error)
}

func (s *PostingService) PostDue(ctx context.Context) (int, error) {
	if s.Scheduler == nil {
		return 0, errors.New("scheduler is required")
	}
	cal, err := s.Scheduler.Load()
	if err != nil {
		return 0, err
	}
	now := time.Now().UTC()
	if s.Now != nil {
		now = s.Now().UTC()
	}

	fired := 0
	for i := range cal.Slots {
		slot := &cal.Slots[i]
		if !slot.Approved || slot.Status == SlotStatusPosted || slot.SlotTime.After(now) {
			continue
		}
		if _, err := s.PostSlug(ctx, slot.Slug, true); err != nil {
			slot.Status = SlotStatusFailed
			slot.Error = err.Error()
			slot.UpdatedAt = now
			continue
		}
		postedAt := now
		slot.Status = SlotStatusPosted
		slot.PostedAt = &postedAt
		slot.Error = ""
		slot.UpdatedAt = now
		fired++
	}

	if err := s.Scheduler.Save(cal); err != nil {
		return fired, err
	}
	return fired, nil
}

func (s *PostingService) PostSlug(ctx context.Context, slug string, confirm bool) ([]PostResult, error) {
	if !confirm {
		return nil, fmt.Errorf("refusing to post without --confirm")
	}
	if s.Store == nil {
		return nil, errors.New("store is required")
	}
	if s.PosterForPlatform == nil {
		return nil, errors.New("poster factory is required")
	}

	meta, err := s.Store.Get(slug)
	if err != nil {
		return nil, err
	}
	socialData, err := s.Store.ReadSocial(slug)
	if err != nil {
		return nil, err
	}

	posts := []SocialPost{}
	if strings.TrimSpace(socialData.XText) != "" {
		posts = append(posts, SocialPost{
			Platform:   "x",
			Text:       socialData.XText,
			ImagePath:  s.Store.HeroPath(slug),
			ArticleURL: strings.TrimSpace(meta.PublishURL),
		})
	}
	if strings.TrimSpace(socialData.LinkedInText) != "" {
		posts = append(posts, SocialPost{
			Platform:   "linkedin",
			Text:       socialData.LinkedInText,
			ImagePath:  s.Store.HeroLinkedInPath(slug),
			ArticleURL: strings.TrimSpace(meta.PublishURL),
		})
	}
	if len(posts) == 0 {
		return nil, fmt.Errorf("no social copy found for %q", slug)
	}

	results := make([]PostResult, 0, len(posts))
	for _, p := range posts {
		if strings.TrimSpace(p.ImagePath) != "" {
			if _, err := os.Stat(p.ImagePath); errors.Is(err, os.ErrNotExist) {
				p.ImagePath = ""
			}
		}

		poster, err := s.PosterForPlatform(ctx, p.Platform)
		if err != nil {
			return nil, err
		}
		res, err := poster.Post(ctx, p)
		if err != nil {
			return nil, err
		}
		results = append(results, res)
		switch p.Platform {
		case "x":
			socialData.XPostURL = res.URL
		case "linkedin":
			socialData.LinkedInURL = res.URL
		}
	}

	if err := s.Store.WriteSocial(slug, socialData); err != nil {
		return nil, err
	}

	if meta.Status == content.StatusScheduled {
		if err := s.Store.Transition(slug, content.StatusPosted, false); err != nil {
			return nil, err
		}
	}

	return results, nil
}
