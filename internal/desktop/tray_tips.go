package desktop

import (
	"errors"

	"github.com/qinqingxu/synchub-for-agents/internal/tray"
)

func WithTrayTips(service *WailsService, preferences *tray.TipPreferences) *WailsService {
	service.trayTips = preferences
	return service
}

func (s *WailsService) TrayTipsEnabled() (bool, error) {
	if s.trayTips == nil {
		return false, errors.New("tray tip preferences are unavailable")
	}
	return s.trayTips.Enabled(), nil
}

func (s *WailsService) SetTrayTipsEnabled(enabled bool) error {
	if s.trayTips == nil {
		return errors.New("tray tip preferences are unavailable")
	}
	return s.trayTips.SetEnabled(enabled)
}
