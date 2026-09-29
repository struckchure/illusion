package input

import "testing"

func TestButtonInput(t *testing.T) {
	in := NewButtonInput[Key]()
	in.Press(1)
	if !in.Pressed(1) || !in.JustPressed(1) || in.JustReleased(1) {
		t.Fatal("press not recorded")
	}

	in.Clear()
	in.Press(1) // still held: not "just" pressed again
	if !in.Pressed(1) || in.JustPressed(1) {
		t.Fatal("held key reported as just pressed")
	}

	in.Clear()
	in.Release(1)
	if in.Pressed(1) || !in.JustReleased(1) {
		t.Fatal("release not recorded")
	}
	if !in.AnyPressed() == false || in.AnyJustPressed(1, 2) {
		t.Fatal("unexpected any-state")
	}
}
