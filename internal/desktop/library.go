package desktop

import (
	"fmt"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/sharedlibrary"
)

func (s *WailsService) LibrarySnapshot() (sharedlibrary.Snapshot, error) {
	if s.core == nil {
		return sharedlibrary.Snapshot{}, fmt.Errorf("desktop service is unavailable")
	}
	s.core.operations.Lock()
	defer s.core.operations.Unlock()
	return cli.LibrarySnapshot(s.core.home)
}

func (s *WailsService) PreviewLibrary(request sharedlibrary.Request) (sharedlibrary.Plan, error) {
	if s.core == nil {
		return sharedlibrary.Plan{}, fmt.Errorf("desktop service is unavailable")
	}
	s.core.operations.Lock()
	defer s.core.operations.Unlock()
	return cli.PreviewLibrary(s.core.home, request)
}

func (s *WailsService) ApplyLibrary(request sharedlibrary.Request, id string) error {
	if s.core == nil {
		return fmt.Errorf("desktop service is unavailable")
	}
	s.core.operations.Lock()
	defer s.core.operations.Unlock()
	return cli.ApplyLibrary(s.core.home, request, id)
}
