package state

import "testing"

func TestClampWidth(t *testing.T) {
	same(t, ClampWidth(200, 1200), float64(MinDock))
	same(t, ClampWidth(500, 1200), 500.0)
	same(t, ClampWidth(1000, 1200), 1200.0-MinDock) // the main area keeps MinDock
	same(t, ClampWidth(500, 500), float64(MinDock)) // too narrow for both
}

func TestDockingHidesTheChatListAgain(t *testing.T) {
	s := NewShell()
	a, b := ChatRef{"p1", "a"}, ChatRef{"p1", "b"}
	s.Dock(a, Docked{"file", "x.csv"})
	s.ListShown = true
	s.Dock(a, Docked{"file", "y.csv"}) // something was docked: the choice stays
	same(t, s.ListShown, true)
	s.Dock(b, Docked{"file", "x.csv"})
	same(t, s.ListShown, false)
	s.CloseFile("p1", "x.csv")
	if _, ok := s.Open[b]; ok {
		t.Fatal("a deleted file stays docked")
	}
	same(t, s.Open[a].Key, "y.csv")
	s.Swap("p1")
	same(t, s.Side["p1"], DockLeft)
	s.Swap("p1")
	same(t, s.Side["p1"], DockRight)
}
