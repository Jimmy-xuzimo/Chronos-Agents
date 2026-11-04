package agent

func (a *Agent) Paused() bool { a.mu.Lock(); defer a.mu.Unlock(); return a.state.Config.Paused }
func (a *Agent) TogglePause() {
	a.runMu.Lock()
	a.mu.Lock()
	old := a.state.Config.Paused
	a.state.Config.Paused = !old
	if a.persist() != nil {
		a.state.Config.Paused = old
	}
	a.mu.Unlock()
	a.runMu.Unlock()
	a.notify()
}
