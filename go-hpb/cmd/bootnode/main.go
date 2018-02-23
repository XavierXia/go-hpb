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

// bootnode runs a bootstrap node for the HPB Discovery Protocol.
package main

import (
	"crypto/ecdsa"
	"flag"
	"fmt"
	"os"

	"github.com/hpb-project/go-hpb/cmd/utils"
	"github.com/hpb-project/go-hpb/crypto"
	"github.com/hpb-project/go-hpb/log"
	"github.com/hpb-project/go-hpb/p2p/discover"
	"github.com/hpb-project/go-hpb/p2p/nat"
	"github.com/hpb-project/go-hpb/p2p/netutil"
)

func main() {
	var (
		listenAddr  = flag.String("addr", ":30301", "listen address for find light nodes")
		genKey      = flag.String("genkey", "", "generate a node key")
		Role        = flag.Uint("role", uint(discover.LightRole), "role type of node")
		writeAddr   = flag.Bool("writeaddress", false, "write out the node's pubkey hash and quit")
		nodeKeyFile = flag.String("nodekey", "", "private key filename")
		nodeKeyHex  = flag.String("nodekeyhex", "", "private key as hex (for testing)")
		natdesc     = flag.String("nat", "none", "port mapping mechanism (any|none|upnp|pmp|extip:<IP>)")
		netrestrict = flag.String("netrestrict", "", "restrict network communication to the given IP networks (CIDR masks)")
		verbosity   = flag.Int("verbosity", int(log.LvlInfo), "log verbosity (0-9)")
		vmodule     = flag.String("vmodule", "", "log verbosity pattern")

		nodeKey *ecdsa.PrivateKey
		err     error
	)
	flag.Parse()

	glogger := log.NewGlogHandler(log.StreamHandler(os.Stderr, log.TerminalFormat(false)))
	glogger.Verbosity(log.Lvl(*verbosity))
	glogger.Vmodule(*vmodule)
	log.Root().SetHandler(glogger)

	natm, err := nat.Parse(*natdesc)
	if err != nil {
		utils.Fatalf("-nat: %v", err)
	}
	switch {
	case *genKey != "":
		nodeKey, err = crypto.GenerateKey()
		if err != nil {
			utils.Fatalf("could not generate key: %v", err)
		}
		if err = crypto.SaveECDSA(*genKey, nodeKey); err != nil {
			utils.Fatalf("%v", err)
		}
		return
	case *nodeKeyFile == "" && *nodeKeyHex == "":
		utils.Fatalf("Use -nodekey or -nodekeyhex to specify a private key")
	case *nodeKeyFile != "" && *nodeKeyHex != "":
		utils.Fatalf("Options -nodekey and -nodekeyhex are mutually exclusive")
	case *nodeKeyFile != "":
		if nodeKey, err = crypto.LoadECDSA(*nodeKeyFile); err != nil {
			utils.Fatalf("-nodekey: %v", err)
		}
	case *nodeKeyHex != "":
		if nodeKey, err = crypto.HexToECDSA(*nodeKeyHex); err != nil {
			utils.Fatalf("-nodekeyhex: %v", err)
		}
	}

	if *writeAddr {
		fmt.Printf("%v\n", discover.PubkeyID(&nodeKey.PublicKey))
		os.Exit(0)
	}

	var restrictList *netutil.Netlist
	if *netrestrict != "" {
		restrictList, err = netutil.ParseNetlist(*netrestrict)
		if err != nil {
			utils.Fatalf("-netrestrict: %v", err)
		}
	}

	if ga, err := discover.ListenUDP(nodeKey, uint8(*Role), *listenAddr, natm, "", restrictList); err != nil {
		utils.Fatalf("%v", err)
	} else {// else only for test

		var nodesTestString = []string{
			// HPB Foundation Go Bootnodes Test
			"enode://6d30b0cae23373449382e76e5a92cba8a096d0c7259cf6160b747e5cf80aa595842da75e44e650465a227ae7179382d47fbba05446c19d28b7c923ca9b3d71bc&4@192.168.31.119:30303",
		}
		var nodesTest []*discover.Node

		for _, url := range nodesTestString {
			node, err := discover.ParseNode(url)
			if err != nil {
				log.Error("Bootstrap URL invalid", "enode", url, "err", err)
				continue
			}
			nodesTest = append(nodesTest, node)
			log.Info("fall back bootNode", "id", node)
		}

		if err := ga.LightTab.SetFallbackNodes(nodesTest); err != nil {
			return
		}
		if err := ga.CommSlice.SetFallbackNodes(nodesTest); err != nil {
			return
		}
	}

	select {}
}
