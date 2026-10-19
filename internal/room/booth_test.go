package room

import (
	"slices"
	"testing"

	"github.com/akkamaze/synqsong/internal/account"
)

const (
	ann account.ID = iota + 1
	bob
	cat
)

func boothOf(djs ...account.ID) Booth {
	b := Booth{}
	for _, dj := range djs {
		b = b.StepUp(dj)
	}

	return b
}

func assertOrder(t *testing.T, b Booth, want ...account.ID) {
	t.Helper()

	if got := b.DJs(); !slices.Equal(got, want) {
		t.Fatalf("booth order = %v, want %v", got, want)
	}
}

func TestStepUpJoinsTheEndOnce(t *testing.T) {
	b := boothOf(ann, bob)
	b = b.StepUp(cat)
	b = b.StepUp(bob)

	assertOrder(t, b, ann, bob, cat)

	if head, ok := b.Head(); !ok || head != ann {
		t.Fatalf("head = %v, %v; want ann", head, ok)
	}
}

func TestPlayingDJStepsDownAfterTheSong(t *testing.T) {
	b := boothOf(ann, bob).StepDown(ann, true)

	assertOrder(t, b, ann, bob)

	if !b.HeadLeaving() {
		t.Fatal("playing DJ who stepped down is not marked to leave after the song")
	}

	b = b.FinishTurn(false)
	assertOrder(t, b, bob)

	if b.HeadLeaving() {
		t.Fatal("next DJ inherited the leaving mark")
	}
}

func TestSteppingBackUpCancelsLeaving(t *testing.T) {
	b := boothOf(ann, bob).StepDown(ann, true).StepUp(ann)

	if b.HeadLeaving() {
		t.Fatal("stepping back up mid-song did not cancel leaving")
	}

	assertOrder(t, b.FinishTurn(false), bob, ann)
}

func TestOnDeckDJStepsDownAtOnce(t *testing.T) {
	assertOrder(t, boothOf(ann, bob, cat).StepDown(bob, true), ann, cat)
}

func TestHeadNotPlayingStepsDownAtOnce(t *testing.T) {
	assertOrder(t, boothOf(ann, bob).StepDown(ann, false), bob)
}

func TestFinishTurnRotatesOrDropsTheHead(t *testing.T) {
	assertOrder(t, boothOf(ann, bob, cat).FinishTurn(false), bob, cat, ann)
	assertOrder(t, boothOf(ann, bob, cat).FinishTurn(true), bob, cat)
}

func TestRotatePastMovesHeadToBack(t *testing.T) {
	assertOrder(t, boothOf(ann, bob, cat).RotatePast(), bob, cat, ann)
}

func TestRemoveDropsAnyoneAndClearsLeaving(t *testing.T) {
	b := boothOf(ann, bob, cat).StepDown(ann, true)

	b = b.Remove(ann)
	assertOrder(t, b, bob, cat)

	if b.HeadLeaving() {
		t.Fatal("removing the head left the leaving mark on the next DJ")
	}

	assertOrder(t, b.Remove(cat), bob)
	assertOrder(t, b.Remove(ann), bob, cat)
}

func TestEmptyBooth(t *testing.T) {
	var b Booth

	if _, ok := b.Head(); ok {
		t.Fatal("empty booth has a head")
	}

	assertOrder(t, b.FinishTurn(false))
	assertOrder(t, b.RotatePast())
	assertOrder(t, b.StepDown(ann, true))
}

func TestBoothChangesNeverTouchTheOriginal(t *testing.T) {
	original := boothOf(ann, bob, cat)

	_ = original.FinishTurn(false)
	_ = original.RotatePast()
	_ = original.Remove(bob)
	_ = original.StepDown(ann, true)
	_ = original.StepUp(4)

	assertOrder(t, original, ann, bob, cat)

	if original.HeadLeaving() {
		t.Fatal("original booth was marked leaving by a change to a copy")
	}
}

func TestRotatePastNeverDropsTheHead(t *testing.T) {
	b := boothOf(ann, bob).StepDown(ann, true).RotatePast()

	assertOrder(t, b, bob, ann)
}
