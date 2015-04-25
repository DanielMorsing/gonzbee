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

type parfile struct {
	n    int
	file *nzb.File
}

var parregexp = regexp.MustCompile(`(?i)(\.vol\d+\+(\d+))?\.par2$`)

func filterPars(nfile *nzb.Nzb) map[*nzb.File][]*parfile {
	// files we should remove
	remove := make(map[*nzb.File]bool)
	// the recover parfiles tied to their parent file
	parsets := make(map[*nzb.File][]*parfile)
	// recovery parfiles tied to the filename prefix.
	parfiles := make(map[string][]*parfile)

	for _, f := range nfile.File {
		filename := f.Subject.Filename()
		s := parregexp.FindStringSubmatch(filename)
		if s == nil {
			continue
		}
		if s[1] != "" {
			numstr := s[2]
			n, err := strconv.Atoi(numstr)
			if err != nil {
				continue
			}
			pfile := &parfile{
				n:    n,
				file: f,
			}
			prefix := filename[:len(filename)-len(s[0])]
			parfiles[prefix] = append(parfiles[prefix], pfile)
		} else {
			parsets[f] = nil
		}
		remove[f] = true
	}

	for fp, _ := range parsets {
		fname := fp.Subject.Filename()
		// We know that these files will have .par2 extensions
		fname = fname[:len(fname)-len(".par2")]
		parsets[fp] = parfiles[fname]
	}

	files := make([]*nzb.File, 0, len(nfile.File))
	for _, f := range nfile.File {
		if !remove[f] {
			files = append(files, f)
		}
	}

	nfile.File = files
	return parsets
}

// selectPars takes a list of parfiles and selects npar blocks from them.
func selectPars(parfiles []*parfile, npars int) []*nzb.File {
	// Selecting the set of par files is fairly tricky.
	// it can be boiled down to an inverted knapsack problem.
	// which is figuring out how many blocks we need to remove
	// in order to get at least npars blocks
	sum := 0
	for _, p := range parfiles {
		sum += p.n
	}
	target := sum - npars

	var files []*nzb.File
	if target < 1 {
		for _, pf := range parfiles {
			files = append(files, pf.file)
		}
		return files
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
	return files
}
