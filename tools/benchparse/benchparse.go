package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
)

var (
	omitMem       = flag.Bool("omit-mem", false, "omit B/op stat")
	omitAllocs    = flag.Bool("omit-allocs", false, "omit allocs/op stat")
	omitBandwidth = flag.Bool("omit-bandwidth", false, "omit MB/s stat")
	flagInput     = flag.String("in", "benchstats.csv", "csv-formatted benchstat file")
	flagOutput    = flag.String("out", "benchstats.json", "json-formatted data table file")
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: benchparse [options]\n")
	fmt.Fprintf(os.Stderr, "options:\n")
	flag.PrintDefaults()
	os.Exit(2)
}

type set map[string][]benchStat

type benchStat struct {
	Name  string
	Value float64
	Unit  string
}

type gcDataTables map[string][][]interface{}

func main() {
	log.SetPrefix("benchparse: ")
	log.SetFlags(0)
	flag.Usage = usage
	flag.Parse()

	file, err := os.Open(*flagInput)
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	stats, err := parse(bufio.NewScanner(file))
	if err != nil {
		log.Fatal(err)
	}
	out, err := os.OpenFile(*flagOutput, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	data := transform(stats)

	enc := json.NewEncoder(out)
	if err := enc.Encode(data); err != nil {
		log.Fatal(err)
	}
	if err := out.Sync(); err != nil {
		log.Fatal(err)
	}
	os.Exit(0)
}

func parse(scanr *bufio.Scanner) (set, error) {
	var (
		i     int
		unit  string
		stats = make(set)
	)
	// The first line is always the header of the
	// first section. All the next section headers
	// will be detected thanks to the empty line
	// that precede them.
	nextIsHdr := true

	for ; scanr.Scan(); i++ {
		line := scanr.Text()

		if len(line) == 0 {
			nextIsHdr = true
			continue
		}
		r := strings.Split(line, ",")

		if nextIsHdr {
			if len(r) != 2 {
				return nil, fmt.Errorf("invalid header at line %d", i)
			}
			s := strings.Split(r[1], " ")[1]
			unit = strings.Trim(s, "()")
			nextIsHdr = false
			continue
		}
		idx := strings.Index(r[0], "/")
		if idx == -1 {
			return nil, fmt.Errorf("invalid run name at line %d: no separator", i)
		}
		bname, rname := r[0][:idx], r[0][idx+1:]
		if idx := strings.LastIndexByte(rname, '-'); idx != -1 {
			rname = rname[:idx]
		}
		if _, ok := stats[bname]; !ok {
			stats[bname] = nil
		}
		if len(r) <= 1 {
			continue
		}
		f, err := strconv.ParseFloat(r[1], 64)
		if err != nil {
			return nil, fmt.Errorf("invalid float value at line %d: %s", i, err)
		}
		stats[bname] = append(stats[bname], benchStat{
			Name:  rname,
			Value: f,
			Unit:  unit,
		})
	}
	if err := scanr.Err(); err != nil {
		return nil, err
	}
	return stats, nil
}

// transform converts a stats set to a 2D data-table
// interpretable by the Google Charts API.
func transform(stats set) gcDataTables {
	data := make(gcDataTables)

	for bname, stats := range stats {
		values := make(map[string][]interface{})
	L:
		for _, s := range stats {
			if _, ok := values[s.Name]; !ok {
				values[s.Name] = append(values[s.Name], s.Name)
			}
			switch s.Unit {
			case "MB/s":
				if *omitBandwidth {
					continue L
				}
			case "B/op": // total memory allocated
				if *omitMem {
					continue L
				}
			case "allocs/op": // number of allocs
				if *omitAllocs {
					continue L
				}
			}
			values[s.Name] = append(values[s.Name], s.Value)
		}
		data[bname] = append(data[bname], []interface{}{"Name", "ns/op"})
		if !*omitBandwidth {
			data[bname][0] = append(data[bname][0], "MB/s")
		}
		if !*omitMem {
			data[bname][0] = append(data[bname][0], "B/op")
		}
		if !*omitAllocs {
			data[bname][0] = append(data[bname][0], "allocs/op")
		}
		for _, v := range values {
			data[bname] = append(data[bname], v)
		}
	}
	return data
}
