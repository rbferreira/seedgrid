package rtorrent

import (
	"context"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"github.com/rbferreira/seedgrid/agent/internal/engine"
	"github.com/rbferreira/seedgrid/agent/internal/engine/queue"
)

// loadedRow is torrentRow with its own d.load_date, the queue's order.
func loadedRow(hash string, running, complete int, custom string, loaded int64) []any {
	r := torrentRow(hash, hash, running, running, complete, 0, "", 100, 50, 100<<10, 0, 0, 0, "/d", 0, custom)
	r[len(r)-1] = loaded
	return r
}

// TestQueueRound: over the limit the newest is stopped, closed and marked; a queued one starts when
// a slot frees. Before the first configuration nothing happens at all.
func TestQueueRound(t *testing.T) {
	f := fakeWithTorrents(
		loadedRow("AAAA", 1, 0, "", 1),
		loadedRow("BBBB", 1, 0, "", 2),
		loadedRow("CCCC", 1, 0, "", 3),
	)
	var got [][]any
	record(f, &got, "d.stop", "d.close", "d.custom.set", "d.open", "d.start")
	a, _ := New(f.serveSCGI(t))
	a.WithQueue(engine.Config{QueueingEnabled: true, MaxActiveDownloads: 1, MaxActiveUploads: -1, MaxActiveTorrents: -1})
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	q := queue.New()

	if err := a.queueRound(context.Background(), q, time.Now(), log); err != nil || len(got) != 0 {
		t.Fatalf("before the first configuration: calls %v, err %v; want none", got, err)
	}

	_ = a.ConfigureQueue(context.Background(), engine.Config{QueueingEnabled: true, MaxActiveDownloads: 2, MaxActiveUploads: -1, MaxActiveTorrents: -1})
	if err := a.queueRound(context.Background(), q, time.Now(), log); err != nil {
		t.Fatal(err)
	}
	want := [][]any{{"d.stop", "CCCC"}, {"d.close", "CCCC"}, {"d.custom.set", "CCCC", customKey, queuedMark}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v", got, want)
	}

	f2 := fakeWithTorrents(loadedRow("AAAA", 1, 0, "", 1), loadedRow("BBBB", 0, 0, queuedMark, 2), loadedRow("CCCC", 0, 0, "", 3))
	got = nil
	record(f2, &got, "d.stop", "d.close", "d.custom.set", "d.open", "d.start")
	a2, _ := New(f2.serveSCGI(t))
	_ = a2.ConfigureQueue(context.Background(), engine.Config{QueueingEnabled: true, MaxActiveDownloads: 2, MaxActiveUploads: -1, MaxActiveTorrents: -1})
	if err := a2.queueRound(context.Background(), queue.New(), time.Now(), log); err != nil {
		t.Fatal(err)
	}
	want = [][]any{{"d.custom.set", "BBBB", customKey, ""}, {"d.open", "BBBB"}, {"d.start", "BBBB"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %v, want %v (CCCC was not stopped by the queue)", got, want)
	}
}

// TestAddFile_Queued: with the queue on, a new torrent comes in stopped and marked, and the queue
// starts it; before the first configuration it starts as usual.
func TestAddFile_Queued(t *testing.T) {
	f := newFake()
	var got [][]any
	record(f, &got, "load.raw", "load.raw_start", "load.normal", "load.start")
	f.on("d.hash", func(p []any) (any, error) { return p[0], nil })
	a, _ := New(f.serveSCGI(t))
	a.WithQueue(engine.Config{QueueingEnabled: true})
	_ = a.AddFile(context.Background(), []byte("d4:infod4:name1:xee"))
	_ = a.ConfigureQueue(context.Background(), engine.Config{QueueingEnabled: true})
	_ = a.AddFile(context.Background(), []byte("d4:infod4:name1:xee"))
	_ = a.AddURL(context.Background(), "https://example.org/x.torrent")
	_ = a.AddURL(context.Background(), "magnet:?xt=urn:btih:abcd")
	want := [][]any{
		{"load.raw_start", "", []byte("d4:infod4:name1:xee")},
		{"load.raw", "", []byte("d4:infod4:name1:xee"), "d.custom.set=seedgrid,queued"},
		{"load.normal", "", "https://example.org/x.torrent", "d.custom.set=seedgrid,queued"},
		{"load.start", "", "magnet:?xt=urn:btih:abcd"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("calls = %#v, want %#v", got, want)
	}
}
