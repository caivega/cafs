//  BitWrk - A Bitcoin-friendly, anonymous marketplace for computing power
//  Copyright (C) 2013-2019 Jonas Eschenburg <jonas@bitwrk.net>
//
//  This program is free software: you can redistribute it and/or modify
//  it under the terms of the GNU General Public License as published by
//  the Free Software Foundation, either version 3 of the License, or
//  (at your option) any later version.
//
//  This program is distributed in the hope that it will be useful,
//  but WITHOUT ANY WARRANTY; without even the implied warranty of
//  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//  GNU General Public License for more details.
//
//  You should have received a copy of the GNU General Public License
//  along with this program.  If not, see <http://www.gnu.org/licenses/>.package main

package main

import (
	"flag"

	"github.com/indyjo/cafs/remotesync"
	"github.com/indyjo/cafs/remotesync/httpsync/cmd"
)

func main() {
	addr := ":8080"
	flag.StringVar(&addr, "l", addr, "which port to listen to")

	preload := ""
	flag.StringVar(&preload, "i", preload, "input file to load")

	dataDir := "."
	flag.StringVar(&dataDir, "d", dataDir, "data dir for upload file")

	flag.BoolVar(&remotesync.LoggingEnabled, "enable-remotesync-logging", remotesync.LoggingEnabled,
		"enables detailed logging from the remotesync algorithm")

	flag.Parse()

	list := []string{}
	if preload != "" {
		list = append(list, preload)
	}
	cmd.Service(addr, dataDir, list)
}
