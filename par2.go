//Copyright 2013, Daniel Morsing
//For licensing information, See the LICENSE file

// code for figuring out which par files to select to get the proper
// number of blocks.

package main

import (
	"github.com/DanielMorsing/gonzbee/nzb"
	"regexp"
	"strconv"
)

var parregexp = regexp.MustCompile(`(?i)\+(\d+).par2$`)

// filter out par2 files containing at least npars blocks.
// if npars is negative, download all blocks.
func filterPars(nfile *nzb.Nzb, npars int) bool {
	files := make([]*nzb.File, 0, len(nfile.File))
	
	type parfile struct {
		n    int
		file *nzb.File
	}

	parfiles := make([]*parfile, 0, len(nfile.File))
	if npars < 0 {
		return true
	}

	// figure out how many par2 files there are and how many blocks they contain.
	// if they're not parfiles, just add them.
	var sum int
	for _, f := range nfile.File {
		s := parregexp.FindStringSubmatch(f.Subject.Filename())
		if s == nil {
			files = append(files, f)
		} else {
			p, err := strconv.ParseInt(s[1], 10, 0)
			if err != nil {
				// be safe and just add this to the download set
				files = append(files, f)
				continue
			}
			sum += int(p)
			parfiles = append(parfiles, &parfile{int(p), f})
		}
	}

	// Selecting the set of par files is fairly tricky.
	// it can be boiled down to an inverted knapsack problem.
	// which is figuring out how many blocks we need to remove
	// in order to get at least npars blocks
	target := sum - npars

	if target < 1 {
		// want more blocks than available, or the exact amount.
		return target == 0
	}

	// the plus one here is not an error. it's to make space for the empty case
	m := make([][]int, len(parfiles)+1)
	keep := make([][]bool, len(parfiles)+1)
	for i := range m {
		// also not an error, it's to make space for both 0 and target inclusively
		m[i] = make([]int, target+1)
		keep[i] = make([]bool, target+1)
	}
	// this is the standard dynamic programming solution for the knapsack problem.
	for i := 1; i <= len(parfiles); i++ {
		w := parfiles[i-1].n
		for j := 0; j <= target; j++ {
			if j >= w {
				oldValue := m[i-1][j]
				potentialVal := m[i-1][j-w] + w
				if oldValue < potentialVal {
					m[i][j] = potentialVal
					keep[i][j] = true
				} else {
					m[i][j] = oldValue
				}
			} else {
				m[i][j] = m[i-1][j]
			}
		}
	}

	// walk through the keep matrix
	excludeSet := make([]*parfile, 0, len(parfiles))
	i, j := len(parfiles), target
	for i > 0 {
		if keep[i][j] {
			j -= parfiles[i-1].n
			excludeSet = append(excludeSet, parfiles[i-1])
		}
		i -= 1
	}

	// this is a bit of a hack. Since i know that excludeset
	// was added to in reverse order, i can do a linear scan here
	// going backwards in excludeset and forwards in parfiles
	// in order select all parfiles that wasn't in excludeset.
	i, j = 0, len(excludeSet)-1
	for j >= 0 {
		if parfiles[i] == excludeSet[j] {
			j--
		} else {
			files = append(files, parfiles[i].file)
		}
		i++
	}
	// add the remaining parfiles to the set.
	for ; i < len(parfiles); i++ {
		files = append(files, parfiles[i].file)
	}
	nfile.File = files
	return true
}

