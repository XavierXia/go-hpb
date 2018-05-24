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

// Contains the metrics collected by the syn.

package synctrl

import (
	"github.com/hpb-project/go-hpb/metrics"
)

var (
	headerInMeter      = metrics.NewMeter("hpb/syn/headers/in")
	headerReqTimer     = metrics.NewTimer("hpb/syn/headers/req")
	headerDropMeter    = metrics.NewMeter("hpb/syn/headers/drop")
	headerTimeoutMeter = metrics.NewMeter("hpb/syn/headers/timeout")

	bodyInMeter      = metrics.NewMeter("hpb/syn/bodies/in")
	bodyReqTimer     = metrics.NewTimer("hpb/syn/bodies/req")
	bodyDropMeter    = metrics.NewMeter("hpb/syn/bodies/drop")
	bodyTimeoutMeter = metrics.NewMeter("hpb/syn/bodies/timeout")

	receiptInMeter      = metrics.NewMeter("hpb/syn/receipts/in")
	receiptReqTimer     = metrics.NewTimer("hpb/syn/receipts/req")
	receiptDropMeter    = metrics.NewMeter("hpb/syn/receipts/drop")
	receiptTimeoutMeter = metrics.NewMeter("hpb/syn/receipts/timeout")

	stateInMeter   = metrics.NewMeter("hpb/syn/states/in")
	stateDropMeter = metrics.NewMeter("hpb/syn/states/drop")
)
