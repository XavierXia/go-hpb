// Copyright 2018 The go-hpb Authors
// This file is part of the go-hpb.
//
// The go-hpb is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-hpb is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-hpb. If not, see <http://www.gnu.org/licenses/>.

// ghpb is the official command-line client for Hpb.
package main

import (
	"fmt"
	"os"
	"github.com/hpb-project/go-hpb/common"
	"gopkg.in/urfave/cli.v1"
	"path/filepath"
	"github.com/hpb-project/go-hpb/config"
	"github.com/hpb-project/go-hpb/core"
	"github.com/hpb-project/go-hpb/txpool"
)


var (

	configFileFlag = cli.StringFlag{
		Name:  "config",
		Usage: "TOML configuration file",
	}
)
// NewApp creates an app with sane defaults.
func NewApp(gitCommit, usage string) *cli.App {
	app := cli.NewApp()
	app.Name = filepath.Base(os.Args[0])
	app.Author = ""
	//app.Authors = nil
	app.Email = ""
	app.Version = config.Version
	if gitCommit != "" {
		app.Version += "-" + gitCommit[:8]
	}
	app.Usage = usage
	return app
}
var (
	// Git SHA1 commit hash of the release (set via linker flags)
	GitCommit = gitCommit
	gitCommit = ""
	// Hpb address of the Geth release oracle.
	relOracle = common.HexToAddress("0xfa7b9770ca4cb04296cac84f37736d4041251cdf")
	// The app that holds all commands and flags.
	app = NewApp(gitCommit, "the go-hpb command line interface")
)

func init() {
	// Initialize the CLI app and start Geth
	app.Action = ghpb
	app.HideVersion = true // we have a command to print the version
	app.Copyright = "Copyright 2013-2018 The go-hpb Authors "
}

func main() {
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ghpb is the main entry point into the system if no special subcommand is ran.
// It creates a default node based on the command line arguments and runs it in
// blocking mode, waiting for it to be shut down.
func ghpb(ctx *cli.Context) error {
	stop := make(chan struct{})
	bc := core.InstanceBlockChain()
	txpool.NewTxPool(config.DefaultTxPoolConfig,config.TestnetChainConfig,bc)
	<-stop
	return nil
}