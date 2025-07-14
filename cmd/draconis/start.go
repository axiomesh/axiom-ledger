package main

import (
	"context"
	"fmt"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"

	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-kit/fileutil"
	"github.com/axiomesh/axiom-ledger/api/jsonrpc"
	"github.com/axiomesh/axiom-ledger/internal/app"
	"github.com/axiomesh/axiom-ledger/internal/coreapi"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/axiomesh/axiom-ledger/pkg/profile"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var startArgs = struct {
	Readonly bool
	Snapshot bool
}{}

func start(ctx *cli.Context) error {
	p, err := getRootPath(ctx)
	if err != nil {
		return err
	}

	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("draconis is not initialized, please execute 'init.sh' first")
		return nil
	}

	r, err := repo.Load(configGenerateArgs.Auth, p, true)
	if err != nil {
		return err
	}
	r.StartArgs = &repo.StartArgs{ReadonlyMode: startArgs.Readonly, SnapshotMode: startArgs.Snapshot}

	appCtx, cancel := context.WithCancel(ctx.Context)
	if err := loggers.Initialize(appCtx, r, true); err != nil {
		cancel()
		return err
	}
	defer cancel()

	log := loggers.Logger(loggers.App)
	printVersion(func(c string) {
		log.Info(c)
	})
	r.PrintNodeInfo(func(c string) {
		log.Info(c)
	})

	runtime.SetMutexProfileFraction(1)
	runtime.SetBlockProfileRate(1)

	var wg sync.WaitGroup
	err = func() error {
		if err := repo.WritePid(r.RepoRoot); err != nil {
			return fmt.Errorf("write pid error: %s", err)
		}

		axm, err := app.NewAxiomLedger(r, appCtx, cancel)
		if err != nil {
			return fmt.Errorf("init draconis failed: %w", err)
		}

		monitor, err := profile.NewMonitor(r.Config)
		if err != nil {
			return err
		}
		if err := monitor.Start(); err != nil {
			return err
		}

		pprof, err := profile.NewPprof(r)
		if err != nil {
			return err
		}
		if err := pprof.Start(); err != nil {
			return err
		}

		// coreapi
		api, err := coreapi.New(axm)
		if err != nil {
			return err
		}

		// start json-rpc service
		cbs, err := jsonrpc.NewChainBrokerService(api, r)
		if err != nil {
			return err
		}

		if err := cbs.Start(); err != nil {
			return fmt.Errorf("start chain broker service failed: %w", err)
		}

		axm.Monitor = monitor
		axm.Pprof = pprof
		axm.Jsonrpc = cbs

		wg.Add(1)
		handleShutdown(axm, &wg)

		if err := axm.Start(); err != nil {
			return fmt.Errorf("start draconis failed: %w", err)
		}

		return nil
	}()
	if err != nil {
		log.WithField("err", err).Error("Startup failed")
		return err
	}

	wg.Wait()

	if err := repo.RemovePID(r.RepoRoot); err != nil {
		log.WithField("err", err).Error("Remove pid failed")
		return fmt.Errorf("remove pid file error: %s", err)
	}

	return nil
}

func printVersion(writer func(c string)) {
	writer(fmt.Sprintf("%s version: %s-%s-%s", repo.AppName, repo.BuildVersion, repo.BuildBranch, repo.BuildCommit))
	writer(fmt.Sprintf("App build date: %s", repo.BuildDate))
	writer(fmt.Sprintf("System version: %s", repo.Platform))
	writer(fmt.Sprintf("Golang version: %s", repo.GoVersion))
}

func handleShutdown(node *app.AxiomLedger, wg *sync.WaitGroup) {
	var stop = make(chan os.Signal, 2)
	signal.Notify(stop, syscall.SIGTERM)
	signal.Notify(stop, syscall.SIGINT)

	go func() {
		<-stop
		fmt.Println("received interrupt signal, shutting down...")
		if err := node.Stop(); err != nil {
			panic(err)
		}
		wg.Done()
		os.Exit(0)
	}()
}
