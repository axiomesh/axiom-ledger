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

	"github.com/axiomesh/axiom-ledger/pkg/profile"
	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-kit/fileutil"
	"github.com/axiomesh/axiom-ledger/api/jsonrpc"
	"github.com/axiomesh/axiom-ledger/cmd/axiom-ledger/common"
	"github.com/axiomesh/axiom-ledger/internal/app"
	"github.com/axiomesh/axiom-ledger/internal/coreapi"
	"github.com/axiomesh/axiom-ledger/pkg/loggers"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
)

var startArgs = struct {
	Readonly bool
	Snapshot bool
	Archive  bool
}{}

var syncPeerArgs = struct {
	remotePeers cli.StringSlice
}{}

func start(ctx *cli.Context) error {
	p, err := common.GetRootPath(ctx)
	if err != nil {
		return err
	}

	if !fileutil.Exist(filepath.Join(p, repo.CfgFileName)) {
		fmt.Println("axiom-ledger is not initialized, please execute 'init.sh' first")
		return nil
	}

	password := common.KeystorePasswordFlagVar
	if !ctx.IsSet(common.KeystorePasswordFlag().Name) {
		password, err = common.EnterPassword(false)
		if err != nil {
			return err
		}
	}

	r, err := repo.Load(p)
	if err != nil {
		return err
	}
	r.StartArgs = &repo.StartArgs{ReadonlyMode: startArgs.Readonly, SnapshotMode: startArgs.Snapshot, ArchiveMode: startArgs.Archive}
	r.SyncArgs = &repo.SyncArgs{}
	if peers := syncPeerArgs.remotePeers.Value(); len(peers) != 0 {
		r.SyncArgs.RemotePeers, err = common.DecodePeers(peers)
		if err != nil {
			return err
		}
	}
	if err := r.ReadKeystore(); err != nil {
		return err
	}

	if password == "" {
		password = repo.DefaultKeystorePassword
		fmt.Println("keystore password is empty, will use default")
	}
	if err := r.DecryptKeystore(password); err != nil {
		return err
	}

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

		axm, err := app.NewAxiomLedger(r, appCtx, cancel)
		if err != nil {
			return fmt.Errorf("init axiom-ledger failed: %w", err)
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

		wg.Add(1)
		handleShutdown(axm, &wg)

		if err := axm.Start(); err != nil {
			return fmt.Errorf("start axiom-ledger failed: %w", err)
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

	stopNode := func(n *app.AxiomLedger) {
		if err := n.Stop(); err != nil {
			panic(err)
		}
		wg.Done()
		os.Exit(0)
	}

	go func() {
		for {
			select {
			case err := <-node.StopCh:
				fmt.Println("received stop signal, shutting down...")
				fmt.Println(err.Error())
				stopNode(node)
			case <-stop:
				fmt.Println("received interrupt signal, shutting down...")
				stopNode(node)
			}
		}
	}()
}
