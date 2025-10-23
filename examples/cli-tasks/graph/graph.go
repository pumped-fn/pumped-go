package graph

import (
	"github.com/pumped-fn/pumped-go/examples/cli-tasks/services"
	"github.com/pumped-fn/pumped-go/examples/cli-tasks/storage"

	pumped "github.com/pumped-fn/pumped-go"
)

type ConfigType struct {
	StorageType string
	FilePath    string
}

var (
	// Configuration (no dependencies)
	Config = pumped.Provide(func(ctx *pumped.ResolveCtx) (*ConfigType, error) {
		return &ConfigType{
			StorageType: "memory",
			FilePath:    "tasks.json",
		}, nil
	})

	// Infrastructure
	Storage = pumped.Derive1(
		Config,
		func(ctx *pumped.ResolveCtx, cfgCtrl *pumped.Controller[*ConfigType]) (storage.Storage, error) {
			cfg, err := cfgCtrl.Get()
			if err != nil {
				return nil, err
			}

			switch cfg.StorageType {
			case "memory":
				return storage.NewMemoryStorage(), nil
			case "file":
				return storage.NewFileStorage(cfg.FilePath)
			default:
				return storage.NewMemoryStorage(), nil
			}
		},
	)

	// Services
	TaskService = pumped.Derive1(
		Storage,
		func(ctx *pumped.ResolveCtx, storageCtrl *pumped.Controller[storage.Storage]) (*services.TaskService, error) {
			store, err := storageCtrl.Get()
			if err != nil {
				return nil, err
			}
			return services.NewTaskService(store), nil
		},
	)

	StatsService = pumped.Derive1(
		Storage,
		func(ctx *pumped.ResolveCtx, storageCtrl *pumped.Controller[storage.Storage]) (*services.StatsService, error) {
			store, err := storageCtrl.Get()
			if err != nil {
				return nil, err
			}
			return services.NewStatsService(store), nil
		},
	)
)
