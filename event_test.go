package illusion

import (
	"slices"
	"testing"
)

type hit struct{ Damage int }

func TestEventsReachReadersBeforeAndAfterSender(t *testing.T) {
	var early, late []int
	sendOnFrame := map[uint64][]int{0: {1, 2}, 1: {3}}

	send := Fn2(func(w *EventWriter[hit], t *Res[Time]) {
		for _, d := range sendOnFrame[t.Get().Frame()] {
			w.Send(hit{d})
		}
	})
	app := New().AddSystems(Update,
		Fn1(func(r *EventReader[hit]) {
			for h := range r.Read() {
				early = append(early, h.Damage)
			}
		}).Before(send),
		send,
		Fn1(func(r *EventReader[hit]) {
			for h := range r.Read() {
				late = append(late, h.Damage)
			}
		}).After(send),
	)
	for range 4 {
		app.Tick(0)
	}

	// The early reader sees each event one frame later, but still exactly once.
	if !slices.Equal(early, []int{1, 2, 3}) || !slices.Equal(late, []int{1, 2, 3}) {
		t.Fatalf("early=%v late=%v", early, late)
	}
}

func TestEventsExpireAfterTwoFrames(t *testing.T) {
	app := New()
	AddEvent[hit](app)
	app.AddSystems(Startup, Fn1(func(w *EventWriter[hit]) { w.Send(hit{1}) }))
	app.Tick(0) // frame 0: sent during startup, still stored
	app.Tick(0) // frame 1: dropped at the start of this frame

	var r EventReader[hit]
	r.InitParam(app.World)
	if !r.IsEmpty() {
		t.Fatalf("expected the event to expire, %d left", r.Len())
	}
}

func TestEventReaderStopEarlyAndClear(t *testing.T) {
	app := New()
	var w EventWriter[hit]
	w.InitParam(app.World)
	var r EventReader[hit]
	r.InitParam(app.World)

	w.SendBatch(hit{1}, hit{2}, hit{3})
	for h := range r.Read() {
		if h.Damage == 2 {
			break
		}
	}
	if r.Len() != 1 {
		t.Fatalf("expected 1 unread after breaking early, got %d", r.Len())
	}
	r.Clear()
	if !r.IsEmpty() {
		t.Fatal("Clear should mark everything read")
	}
}
