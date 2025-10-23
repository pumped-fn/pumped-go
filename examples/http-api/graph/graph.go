package graph

import (
	"github.com/pumped-fn/pumped-go/examples/http-api/services"
	"github.com/pumped-fn/pumped-go/examples/http-api/storage"

	pumped "github.com/pumped-fn/pumped-go"
)

type ConfigType struct {
	MaxUsersCache   int
	RateLimitPerMin int
}

var (
	// Configuration (no dependencies)
	Config = pumped.Provide(func(ctx *pumped.ResolveCtx) (*ConfigType, error) {
		return &ConfigType{
			MaxUsersCache:   100,
			RateLimitPerMin: 60,
		}, nil
	})

	// Infrastructure
	Storage = pumped.Provide(func(ctx *pumped.ResolveCtx) (storage.Storage, error) {
		return storage.NewMemoryStorage(), nil
	})

	// Services
	UserService = pumped.Derive1(
		Storage,
		func(ctx *pumped.ResolveCtx, storageCtrl *pumped.Controller[storage.Storage]) (*services.UserService, error) {
			store, err := storageCtrl.Get()
			if err != nil {
				return nil, err
			}
			return services.NewUserService(store), nil
		},
	)

	PostService = pumped.Derive2(
		Storage,
		UserService,
		func(ctx *pumped.ResolveCtx,
			storageCtrl *pumped.Controller[storage.Storage],
			userServiceCtrl *pumped.Controller[*services.UserService]) (*services.PostService, error) {
			store, err := storageCtrl.Get()
			if err != nil {
				return nil, err
			}
			userSvc, err := userServiceCtrl.Get()
			if err != nil {
				return nil, err
			}
			return services.NewPostService(store, userSvc), nil
		},
	)

	StatsService = pumped.Derive2(
		Storage,
		Config.Reactive(),
		func(ctx *pumped.ResolveCtx,
			storageCtrl *pumped.Controller[storage.Storage],
			configCtrl *pumped.Controller[*ConfigType]) (*services.StatsService, error) {
			store, err := storageCtrl.Get()
			if err != nil {
				return nil, err
			}
			cfg, err := configCtrl.Get()
			if err != nil {
				return nil, err
			}
			return services.NewStatsService(store, cfg.MaxUsersCache), nil
		},
	)
)
