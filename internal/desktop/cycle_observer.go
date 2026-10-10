package desktop

import (
	"sync"

	"github.com/qinqingxu/synchub-for-agents/internal/daemon"
)

// SubscribeCycle includes failed and paused cycles that have no completion progress event.
func (s *Service) SubscribeCycle(callback func(daemon.CycleResult)) func() {
	s.mu.Lock()
	id := s.nextObserverID
	s.nextObserverID++
	s.cycleObservers[id] = callback
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			delete(s.cycleObservers, id)
			s.mu.Unlock()
		})
	}
}
