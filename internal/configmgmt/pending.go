package configmgmt

import (
	"sync"

	"printmarshal/internal/model"
)

// pendingChanges keeps the un-redacted changes of submitted jobs until the
// runner picks them up, so a persisted job never carries a secret value while
// the write still receives it. Jobs that outlive the process (restart) are
// marked interrupted by the runner and must be re-submitted.
type pendingChanges struct {
	mu sync.Mutex
	m  map[*model.Job][]model.SettingChange
}

func (p *pendingChanges) Store(j *model.Job, ch []model.SettingChange) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m == nil {
		p.m = map[*model.Job][]model.SettingChange{}
	}
	p.m[j] = ch
}

func (p *pendingChanges) Delete(j *model.Job) {
	p.mu.Lock()
	delete(p.m, j)
	p.mu.Unlock()
}

func (s *Service) takePending(j *model.Job) []model.SettingChange {
	s.pending.mu.Lock()
	defer s.pending.mu.Unlock()
	ch, ok := s.pending.m[j]
	if ok {
		delete(s.pending.m, j)
	}
	return ch
}
