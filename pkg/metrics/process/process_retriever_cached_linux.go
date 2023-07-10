// Copyright New Relic Corporation. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

//go:build linux

package process

import (
	"strconv"
	"strings"
)

func (s *ProcessRetrieverCached) retrieveProcesses(psBin string) (map[int32]psItem, error) {
	// get all processes info
	args := []string{"ax", "-o", "uid,pid,ppid,user,state,utime,stime,etime,rss,vsize,pagein,ucmd"}
	out, err := commandRunner(psBin, "", args...)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(out, "\n")
	items := make(map[int32]psItem)
	for _, line := range lines[1:] {
		var lineItems []string
		for _, lineItem := range strings.Split(line, " ") {
			if lineItem == "" {
				continue
			}
			lineItems = append(lineItems, strings.TrimSpace(lineItem))
		}
		if len(lineItems) > 10 {
			uid, _ := strconv.Atoi(lineItems[0])
			pid, _ := strconv.Atoi(lineItems[1])
			ppid, _ := strconv.Atoi(lineItems[2])
			user := lineItems[3]
			state := lineItems[4]
			utime := lineItems[5]
			stime := lineItems[6]
			etime := lineItems[7]
			rss, _ := strconv.ParseInt(lineItems[8], 10, 64)
			vsize, _ := strconv.ParseInt(lineItems[9], 10, 64)
			pagein, _ := strconv.ParseInt(lineItems[10], 10, 64)
			command := strings.Join(lineItems[11:], " ")

			item := psItem{
				uid:      int32(uid),
				pid:      int32(pid),
				ppid:     int32(ppid),
				username: user,
				state:    []string{convertStateToGopsutilState(state[0:1])},
				utime:    utime,
				stime:    stime,
				etime:    etime,
				rss:      rss,
				vsize:    vsize,
				pagein:   pagein,
				command:  command,
			}
			items[int32(pid)] = item
		} else {
			mplog.WithField("ps_output", out).Error("ps output is expected to have >10 columns")
		}
	}
	return items, nil
}

func (s *ProcessRetrieverCached) getProcessThreads(psBin string) (map[int32]int32, error) {
	// get all processes info with threads
	args := []string{"-eLf"}
	out, err := commandRunner(psBin, "", args...)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(out, "\n")
	processThreads := make(map[int32]int32)
	for _, line := range lines[1:] {
		for _, lineItem := range strings.Split(line, " ") {
			if lineItem == "" {
				continue
			}
			pidAsInt, err := strconv.Atoi(strings.TrimSpace(lineItem))
			if err != nil {
				mplog.Warnf("pid %v doesn't look like an int", pidAsInt)
				continue
			}
			pid := int32(pidAsInt)
			if _, ok := processThreads[pid]; !ok {
				processThreads[pid] = 0 // main process already included
			}
			processThreads[pid]++
			// we are only interested in pid so break and process next line
			break
		}
	}

	return processThreads, nil
}
