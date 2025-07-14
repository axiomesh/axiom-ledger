package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path"

	"github.com/axiomesh/axiom-ledger/internal/storagemgr"
	"github.com/axiomesh/axiom-ledger/pkg/repo"
	"github.com/urfave/cli/v2"

	"github.com/axiomesh/axiom-kit/fileutil"
	"github.com/axiomesh/axiom-kit/types"
	"github.com/axiomesh/axiom-ledger/internal/txpool"
)

var decodeTxPoolPath string

var txpoolCMD = &cli.Command{
	Name:  "txpool",
	Usage: "The txpool manage commands",
	Subcommands: []*cli.Command{
		{
			Name:  "txrecords",
			Usage: "Get all txs in txrecords",
			Flags: []cli.Flag{
				&cli.StringFlag{
					Name:        "path",
					Aliases:     []string{"p"},
					Usage:       "directory to store txRecords which decoded with JSON",
					Destination: &decodeTxPoolPath,
					Required:    false,
				},
			},
			Action: getAllTxRecords,
		},
	},
}

func getAllTxRecords(ctx *cli.Context) error {
	r, err := prepareRepo(ctx)
	if err != nil {
		return err
	}
	p := path.Join(repo.GetStoragePath(r.RepoRoot), storagemgr.TxPool, txpool.TxRecordsFile)
	if !fileutil.Exist(p) {
		err = fmt.Errorf("draconis is not starting, please run draconis first, " + p)
		return err
	}

	// open the decodeTxPool file for writing
	if decodeTxPoolPath == "" {
		decodeTxPoolPath = path.Join(repo.GetStoragePath(r.RepoRoot), storagemgr.TxPool, txpool.DecodeTxRecordsFile)
	}

	if !fileutil.ExistDir(path.Dir(decodeTxPoolPath)) {
		err = os.MkdirAll(path.Dir(decodeTxPoolPath), 0755)
		if err != nil {
			return err
		}
	}

	// remove old file
	if fileutil.Exist(decodeTxPoolPath) {
		err = os.Remove(decodeTxPoolPath)
		if err != nil {
			return err
		}
	}
	// write new file
	file, err := os.Create(decodeTxPoolPath)
	if err != nil {
		return err
	}
	defer file.Close()

	records, err := txpool.GetAllTxRecords(p)
	if err != nil {
		return err
	}
	for _, record := range records {
		tx := &types.Transaction{}
		err = tx.RbftUnmarshal(record)
		if err != nil {
			continue
		}
		data, err := tx.MarshalJSON()
		if err != nil {
			return err
		}

		var formattedData bytes.Buffer
		err = json.Indent(&formattedData, data, "", "  ")
		if err != nil {
			return err
		}
		if _, err = file.Write(formattedData.Bytes()); err != nil {
			return err
		}
	}
	fmt.Println("success get all txs in txrecords, count: ", len(records))

	return nil
}
