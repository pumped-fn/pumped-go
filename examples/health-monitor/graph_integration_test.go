package main

import (
	"testing"

	pumped "github.com/pumped-fn/pumped-go"
)

func TestGraph_ConfigReactivity(t *testing.T) {
	initialConfig := &Config{
		DBPath:     ":memory:",
		ServerPort: 8080,
		LogLevel:   "info",
	}

	testScope := pumped.NewScope(
		pumped.WithPreset(ConfigExec, initialConfig),
	)
	defer testScope.Dispose()

	db1, err := pumped.Resolve(testScope, DBExec)
	if err != nil {
		t.Fatalf("failed to resolve initial DB: %v", err)
	}

	logger1, err := pumped.Resolve(testScope, LoggerExec)
	if err != nil {
		t.Fatalf("failed to resolve initial logger: %v", err)
	}

	if logger1.level != LogLevelInfo {
		t.Errorf("expected initial logger level to be Info, got %v", logger1.level)
	}

	configAcc := pumped.Accessor(testScope, ConfigExec)
	newConfig := &Config{
		DBPath:     ":memory:",
		ServerPort: 9090,
		LogLevel:   "debug",
	}

	err = configAcc.Update(newConfig)
	if err != nil {
		t.Fatalf("failed to update config: %v", err)
	}

	db2, err := pumped.Resolve(testScope, DBExec)
	if err != nil {
		t.Fatalf("failed to resolve updated DB: %v", err)
	}

	logger2, err := pumped.Resolve(testScope, LoggerExec)
	if err != nil {
		t.Fatalf("failed to resolve updated logger: %v", err)
	}

	if db1 == db2 {
		t.Error("expected DB to be reinitialized after config change")
	}

	if logger1 == logger2 {
		t.Error("expected Logger to be reinitialized after config change")
	}

	if logger2.level != LogLevelDebug {
		t.Errorf("expected updated logger level to be Debug, got %v", logger2.level)
	}
}

func TestGraph_AllComponentsResolve(t *testing.T) {
	testConfig := &Config{
		DBPath:     ":memory:",
		ServerPort: 8080,
		LogLevel:   "info",
	}

	testScope := pumped.NewScope(
		pumped.WithPreset(ConfigExec, testConfig),
	)
	defer testScope.Dispose()

	tests := []struct {
		name string
		fn   func() error
	}{
		{"Logger", func() error {
			_, err := pumped.Resolve(testScope, LoggerExec)
			return err
		}},
		{"DB", func() error {
			_, err := pumped.Resolve(testScope, DBExec)
			return err
		}},
		{"ServiceRepo", func() error {
			_, err := pumped.Resolve(testScope, ServiceRepoExec)
			return err
		}},
		{"HealthRepo", func() error {
			_, err := pumped.Resolve(testScope, HealthRepoExec)
			return err
		}},
		{"IncidentRepo", func() error {
			_, err := pumped.Resolve(testScope, IncidentRepoExec)
			return err
		}},
		{"HealthChecker", func() error {
			_, err := pumped.Resolve(testScope, HealthCheckerExec)
			return err
		}},
		{"IncidentDetector", func() error {
			_, err := pumped.Resolve(testScope, IncidentDetectorExec)
			return err
		}},
		{"Scheduler", func() error {
			_, err := pumped.Resolve(testScope, SchedulerExec)
			return err
		}},
		{"ServiceHandler", func() error {
			_, err := pumped.Resolve(testScope, ServiceHandlerExec)
			return err
		}},
		{"HealthHandler", func() error {
			_, err := pumped.Resolve(testScope, HealthHandlerExec)
			return err
		}},
		{"IncidentHandler", func() error {
			_, err := pumped.Resolve(testScope, IncidentHandlerExec)
			return err
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := tt.fn(); err != nil {
				t.Errorf("failed to resolve %s: %v", tt.name, err)
			}
		})
	}
}

