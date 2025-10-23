package main

import (
	"database/sql"

	pumped "github.com/pumped-fn/pumped-go"
)

var (
	// Configuration (no dependencies)
	ConfigExec = pumped.Provide(func(ctx *pumped.ResolveCtx) (*Config, error) {
		return DefaultConfig(), nil
	})

	// Infrastructure - Logger
	LoggerExec = pumped.Derive1(
		ConfigExec.Reactive(),
		func(ctx *pumped.ResolveCtx, cfgCtrl *pumped.Controller[*Config]) (*Logger, error) {
			cfg, err := cfgCtrl.Get()
			if err != nil {
				return nil, err
			}
			return NewLogger(cfg.LogLevel, nil), nil
		},
	)

	// Infrastructure - Database
	DBExec = pumped.Derive1(
		ConfigExec.Reactive(),
		func(ctx *pumped.ResolveCtx, cfgCtrl *pumped.Controller[*Config]) (*sql.DB, error) {
			cfg, err := cfgCtrl.Get()
			if err != nil {
				return nil, err
			}
			database, err := NewDB(cfg.DBPath)
			if err != nil {
				return nil, err
			}

			ctx.OnCleanup(func() error {
				return database.Close()
			})

			return database, nil
		},
	)

	// Repositories
	ServiceRepoExec = pumped.Derive1(
		DBExec,
		func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (ServiceRepo, error) {
			database, err := dbCtrl.Get()
			if err != nil {
				return nil, err
			}
			return NewServiceRepository(database), nil
		},
	)

	HealthRepoExec = pumped.Derive1(
		DBExec,
		func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (HealthCheckRepo, error) {
			database, err := dbCtrl.Get()
			if err != nil {
				return nil, err
			}
			return NewHealthCheckRepository(database), nil
		},
	)

	IncidentRepoExec = pumped.Derive1(
		DBExec,
		func(ctx *pumped.ResolveCtx, dbCtrl *pumped.Controller[*sql.DB]) (IncidentRepo, error) {
			database, err := dbCtrl.Get()
			if err != nil {
				return nil, err
			}
			return NewIncidentRepository(database), nil
		},
	)

	// Core Services
	HealthCheckerExec = pumped.Provide(func(ctx *pumped.ResolveCtx) (*HealthChecker, error) {
		return NewHealthChecker(), nil
	})

	IncidentDetectorExec = pumped.Derive1(
		IncidentRepoExec,
		func(ctx *pumped.ResolveCtx, repoCtrl *pumped.Controller[IncidentRepo]) (*IncidentDetector, error) {
			repo, err := repoCtrl.Get()
			if err != nil {
				return nil, err
			}
			return NewIncidentDetector(repo), nil
		},
	)

	SchedulerExec = pumped.Derive5(
		ServiceRepoExec,
		HealthRepoExec,
		HealthCheckerExec,
		IncidentDetectorExec,
		LoggerExec,
		func(ctx *pumped.ResolveCtx,
			srCtrl *pumped.Controller[ServiceRepo],
			hrCtrl *pumped.Controller[HealthCheckRepo],
			hcCtrl *pumped.Controller[*HealthChecker],
			idCtrl *pumped.Controller[*IncidentDetector],
			logCtrl *pumped.Controller[*Logger]) (*Scheduler, error) {
			sr, _ := srCtrl.Get()
			hr, _ := hrCtrl.Get()
			hc, _ := hcCtrl.Get()
			id, _ := idCtrl.Get()
			log, _ := logCtrl.Get()

			sched := NewScheduler(sr, hr, hc, id, log)
			sched.Start()

			ctx.OnCleanup(func() error {
				sched.Stop()
				return nil
			})

			return sched, nil
		},
	)

	// HTTP Handlers
	ServiceHandlerExec = pumped.Derive2(
		ServiceRepoExec,
		HealthRepoExec,
		func(ctx *pumped.ResolveCtx,
			srCtrl *pumped.Controller[ServiceRepo],
			hrCtrl *pumped.Controller[HealthCheckRepo]) (*ServiceHandler, error) {
			sr, _ := srCtrl.Get()
			hr, _ := hrCtrl.Get()
			return NewServiceHandler(sr, hr), nil
		},
	)

	HealthHandlerExec = pumped.Derive3(
		ServiceRepoExec,
		HealthRepoExec,
		HealthCheckerExec,
		func(ctx *pumped.ResolveCtx,
			srCtrl *pumped.Controller[ServiceRepo],
			hrCtrl *pumped.Controller[HealthCheckRepo],
			hcCtrl *pumped.Controller[*HealthChecker]) (*HealthHandler, error) {
			sr, _ := srCtrl.Get()
			hr, _ := hrCtrl.Get()
			hc, _ := hcCtrl.Get()
			return NewHealthHandler(sr, hr, hc), nil
		},
	)

	IncidentHandlerExec = pumped.Derive1(
		IncidentRepoExec,
		func(ctx *pumped.ResolveCtx, irCtrl *pumped.Controller[IncidentRepo]) (*IncidentHandler, error) {
			ir, _ := irCtrl.Get()
			return NewIncidentHandler(ir), nil
		},
	)
)
