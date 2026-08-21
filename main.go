// dim-weight-checker reads package dimensions and weight from CSV and
// prints the billable weight a carrier would actually charge for, which
// is the greater of actual weight and dimensional (volumetric) weight.
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

// Divisors are the standard US domestic figures used by UPS, FedEx, and
// USPS Priority Mail: cubic inches per pound for imperial, cubic
// centimeters per kilogram for metric. Individual carrier contracts do
// vary, which is why -divisor exists to override these.
const (
	imperialDivisor = 139.0
	metricDivisor   = 5000.0
)

type record struct {
	id           string
	length       float64
	width        float64
	height       float64
	actualWeight float64
}

func main() {
	unit := flag.String("unit", "in", "measurement system: in (inches/lb) or cm (centimeters/kg)")
	divisor := flag.Float64("divisor", 0, "dim weight divisor, overrides the default for the chosen unit")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-unit in|cm] [-divisor N] [file ...]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Reads shipping package rows (id,length,width,height,weight) as CSV\n")
		fmt.Fprintf(os.Stderr, "and prints the billable weight a carrier would charge for each one.\n")
		fmt.Fprintf(os.Stderr, "With no files given, reads from stdin. Use \"-\" for stdin among files.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	d := *divisor
	if d == 0 {
		if *unit == "cm" {
			d = metricDivisor
		} else {
			d = imperialDivisor
		}
	}

	sources, err := openSources(flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer sources.close()

	w := csv.NewWriter(os.Stdout)
	defer w.Flush()
	w.Write([]string{"id", "length", "width", "height", "actual_weight", "dim_weight", "billable_weight", "dim_applies"})

	exitCode := 0
	for _, src := range sources.readers {
		recs, err := readRecords(src)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			exitCode = 1
			continue
		}
		for _, r := range recs {
			dimWeight := math.Ceil((r.length * r.width * r.height) / d)
			actual := math.Ceil(r.actualWeight)
			billable := actual
			applies := false
			if dimWeight > actual {
				billable = dimWeight
				applies = true
			}
			w.Write([]string{
				r.id,
				strconv.FormatFloat(r.length, 'f', -1, 64),
				strconv.FormatFloat(r.width, 'f', -1, 64),
				strconv.FormatFloat(r.height, 'f', -1, 64),
				strconv.FormatFloat(r.actualWeight, 'f', -1, 64),
				strconv.FormatFloat(dimWeight, 'f', -1, 64),
				strconv.FormatFloat(billable, 'f', -1, 64),
				strconv.FormatBool(applies),
			})
		}
	}
	os.Exit(exitCode)
}

// sourceSet bundles the readers a run should consume, plus any files that
// need closing afterward. Stdin is never closed here.
type sourceSet struct {
	readers []io.Reader
	closers []io.Closer
}

func (s *sourceSet) close() {
	for _, c := range s.closers {
		c.Close()
	}
}

func openSources(paths []string) (*sourceSet, error) {
	if len(paths) == 0 {
		return &sourceSet{readers: []io.Reader{os.Stdin}}, nil
	}
	s := &sourceSet{}
	for _, p := range paths {
		if p == "-" {
			s.readers = append(s.readers, os.Stdin)
			continue
		}
		f, err := os.Open(p)
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", p, err)
		}
		s.readers = append(s.readers, f)
		s.closers = append(s.closers, f)
	}
	return s, nil
}

// readRecords parses id,length,width,height,weight rows. A leading header
// row is detected by checking whether its length column parses as a
// number, and skipped if not; malformed data rows are skipped with a
// warning rather than aborting the whole batch.
func readRecords(r io.Reader) ([]record, error) {
	cr := csv.NewReader(r)
	cr.FieldsPerRecord = -1
	rows, err := cr.ReadAll()
	if err != nil {
		return nil, err
	}

	var recs []record
	for i, row := range rows {
		if len(row) < 5 {
			continue
		}
		if i == 0 {
			if _, err := strconv.ParseFloat(strings.TrimSpace(row[1]), 64); err != nil {
				continue
			}
		}

		length, err1 := strconv.ParseFloat(strings.TrimSpace(row[1]), 64)
		width, err2 := strconv.ParseFloat(strings.TrimSpace(row[2]), 64)
		height, err3 := strconv.ParseFloat(strings.TrimSpace(row[3]), 64)
		weight, err4 := strconv.ParseFloat(strings.TrimSpace(row[4]), 64)
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			fmt.Fprintf(os.Stderr, "skipping malformed row: %v\n", row)
			continue
		}

		recs = append(recs, record{
			id:           strings.TrimSpace(row[0]),
			length:       length,
			width:        width,
			height:       height,
			actualWeight: weight,
		})
	}
	return recs, nil
}
