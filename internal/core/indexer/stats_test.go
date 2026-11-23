package indexer

import (
	"context"
	"log/slog"
	"testing"
	"time"
)

func newTestStats(t *testing.T, ids ...string) (*Stats, *time.Time) {
	t.Helper()
	q := newTestDB(t)
	for _, id := range ids {
		createTestIndexer(t, q, id, "Indexer "+id)
	}
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	s := NewStats(q, slog.Default())
	s.now = func() time.Time { return now }
	return s, &now
}

func TestStats_SummarizeSearches(t *testing.T) {
	ctx := context.Background()
	s, now := newTestStats(t, "a")

	at := *now
	s.RecordSearch(ctx, SearchEvent{IndexerID: "a", At: at, Duration: 1 * time.Second, Seeders: []int{50, 10, 0}})
	s.RecordSearch(ctx, SearchEvent{IndexerID: "a", At: at.Add(time.Minute), Duration: 3 * time.Second, Seeders: []int{100}, Challenged: true, Solved: true})
	s.RecordSearch(ctx, SearchEvent{IndexerID: "a", At: at.Add(2 * time.Minute), Duration: 2 * time.Second, Seeders: nil})
	s.RecordSearch(ctx, SearchEvent{IndexerID: "a", At: at.Add(3 * time.Minute), Duration: 90 * time.Second, ErrorKind: "timeout", Challenged: true})

	got, err := s.Summarize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st := got["a"]
	if st == nil {
		t.Fatal("no stats for indexer a")
	}
	if st.Searches != 4 || st.Failures != 1 {
		t.Errorf("searches/failures = %d/%d, want 4/1", st.Searches, st.Failures)
	}
	if st.SuccessRate != 0.75 {
		t.Errorf("success rate = %v, want 0.75", st.SuccessRate)
	}
	// Durations of successful searches only: 1s, 2s, 3s.
	if st.MedianMs != 2000 || st.P90Ms != 3000 {
		t.Errorf("median/p90 = %d/%d ms, want 2000/3000", st.MedianMs, st.P90Ms)
	}
	if st.AvgResults != 4.0/3 {
		t.Errorf("avg results = %v, want 4/3", st.AvgResults)
	}
	if st.AvgWithSeeders != 1 {
		t.Errorf("avg with seeders = %v, want 1", st.AvgWithSeeders)
	}
	// Per-search medians of searches with results: 10 (of 50,10,0) and 100.
	if st.MedianSeeders != 10 {
		t.Errorf("median seeders = %d, want 10", st.MedianSeeders)
	}
	if st.BehindCloudflare != 0.5 || st.SolveRate != 0.5 {
		t.Errorf("cloudflare/solve = %v/%v, want 0.5/0.5", st.BehindCloudflare, st.SolveRate)
	}
	if st.ErrorKinds["timeout"] != 1 {
		t.Errorf("error kinds = %v, want one timeout", st.ErrorKinds)
	}
	if st.LastSearch != at.Add(3*time.Minute).Format(time.RFC3339) {
		t.Errorf("last search = %q", st.LastSearch)
	}
}

func TestStats_UniqueResults(t *testing.T) {
	ctx := context.Background()
	s, now := newTestStats(t, "a", "b", "c")

	// a finds x,y,z; b finds y,z; c finds z,w. Unique: a=1 (x), b=0, c=1 (w).
	s.NoteResults("a", "dune", []string{"x", "y", "z"})
	s.NoteResults("b", "dune", []string{"y", "z"})
	s.NoteResults("c", "dune", []string{"z", "w"})
	// A query only one indexer answered has nothing to compare against.
	s.NoteResults("a", "alien", []string{"q"})

	// Still inside the window: nothing finalized yet.
	s.FlushOverlap(ctx)
	if got, _ := s.Summarize(ctx); len(got) != 0 {
		t.Fatalf("finalized before the window closed: %v", got)
	}

	*now = now.Add(overlapWindow + time.Second)
	s.FlushOverlap(ctx)
	got, err := s.Summarize(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct {
		unique float64
		avg    float64
	}{
		"a": {1.0 / 3, 1},
		"b": {0, 0},
		"c": {0.5, 1},
	}
	for id, w := range want {
		st := got[id]
		if st == nil || st.Comparisons != 1 {
			t.Fatalf("%s: comparisons = %+v, want 1", id, st)
		}
		if st.UniqueShare != w.unique || st.AvgUnique != w.avg {
			t.Errorf("%s: unique share/avg = %v/%v, want %v/%v", id, st.UniqueShare, st.AvgUnique, w.unique, w.avg)
		}
	}
	s.mu.Lock()
	left := len(s.groups)
	s.mu.Unlock()
	if left != 0 {
		t.Errorf("%d groups left after flush, want 0", left)
	}
}

func TestStats_RepeatedAnswersMerge(t *testing.T) {
	ctx := context.Background()
	s, now := newTestStats(t, "a", "b")
	// A retry (e.g. a cache hit after a timeout) must not double count.
	s.NoteResults("a", "dune", []string{"x"})
	s.NoteResults("a", "dune", []string{"x", "y"})
	s.NoteResults("b", "dune", []string{"x"})
	*now = now.Add(overlapWindow + time.Second)
	s.FlushOverlap(ctx)
	got, _ := s.Summarize(ctx)
	if st := got["a"]; st == nil || st.AvgUnique != 1 || st.UniqueShare != 0.5 {
		t.Errorf("a = %+v, want 1 unique of 2", st)
	}
}

func TestStats_PruneDropsOldRows(t *testing.T) {
	ctx := context.Background()
	s, now := newTestStats(t, "a")
	s.RecordSearch(ctx, SearchEvent{IndexerID: "a", At: now.Add(-statsRetention - time.Hour), Duration: time.Second})
	s.RecordSearch(ctx, SearchEvent{IndexerID: "a", At: *now, Duration: time.Second})
	s.Prune(ctx)
	got, _ := s.Summarize(ctx)
	if st := got["a"]; st == nil || st.Searches != 1 {
		t.Errorf("after prune = %+v, want 1 search", st)
	}
}

func TestTopMedianAndPercentile(t *testing.T) {
	if got := topMedian([]int{1, 2, 3, 100, 200}, 3); got != 100 {
		t.Errorf("topMedian = %d, want 100 (median of 200,100,3)", got)
	}
	if got := percentile([]int{5}, 90); got != 5 {
		t.Errorf("percentile of one = %d", got)
	}
	if got := percentile(nil, 50); got != 0 {
		t.Errorf("percentile of none = %d", got)
	}
}
