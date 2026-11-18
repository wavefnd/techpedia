package support

import "context"

type Store interface {
	GetSupportSettings(ctx context.Context) (Settings, error)
	SaveSupportSettings(ctx context.Context, settings Settings) (Settings, error)
}
