// dim-weight-checker reads package dimensions and weight from CSV and
// prints the billable weight a carrier would actually charge for, which
// is the greater of actual weight and dimensional (volumetric) weight.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"strconv"
	"strings"
)

// Default divisors, used when no -carrier is given: cubic inches per pound
// for imperial, cubic centimeters per kilogram for metric. These match
// UPS and FedEx's published domestic figures. Individual carrier contracts
// do vary, which is why -divisor and -carrier both exist to override them.
const (
	imperialDivisor = 139.0
	metricDivisor   = 5000.0
)

// carrierDivisors holds published dim weight divisors per carrier, for
// carriers whose figures differ from the plain -unit default above. USPS
// has historically used 166 for Priority Mail and Retail Ground rather
// than 139, which works out in the shipper's favor (a higher divisor means
// a lower dim weight for the same box).
var carrierDivisors = map[string]struct {
	imperial float64
	metric   float64
}{
	"ups":   {imperialDivisor, metricDivisor},
	"fedex": {imperialDivisor, metricDivisor},
	"usps":  {166.0, metricDivisor},
}

type record struct {
	id           string
	length       float64
	width        float64
	height       float64
	actualWeight float64
}

func main() {
	unit := flag.String("unit", "in", "measurement system: in (inches/lb) or cm (centimeters/kg)")
	carrier := flag.String("carrier", "", "known carrier to use published divisors for: ups, fedex, usps (overrides the -unit default, overridden by -divisor)")
	divisor := flag.Float64("divisor", 0, "dim weight divisor, overrides the default for the chosen unit and any -carrier")
	jsonInput := flag.Bool("json", false, "parse input as a JSON array of objects instead of CSV")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: %s [-unit in|cm] [-carrier ups|fedex|usps] [-divisor N] [-json] [file ...]\n\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Reads shipping package rows (id,length,width,height,weight) as CSV,\n")
		fmt.Fprintf(os.Stderr, "or as JSON objects with -json, and prints the billable weight a\n")
		fmt.Fprintf(os.Stderr, "carrier would charge for each one.\n")
		fmt.Fprintf(os.Stderr, "With no files given, reads from stdin. Use \"-\" for stdin among files.\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	d, err := divisorFor(*carrier, *unit, *divisor)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
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
		var recs []record
		var err error
		if *jsonInput {
			recs, err = readJSONRecords(src)
		} else {
			recs, err = readRecords(src)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			exitCode = 1
			continue
		}
		for _, r := range recs {
			dimWeight, actual, billable, applies := billableWeight(r, d)
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

// divisorFor resolves the dim weight divisor to use, in priority order:
// an explicit -divisor override, then a known carrier's published figure,
// then the plain -unit default. An unrecognized carrier name is an error
// rather than a silent fallback, since guessing wrong here misreports what
// a shipper will actually be billed.
func divisorFor(carrierName, unit string, override float64) (float64, error) {
	if override != 0 {
		return override, nil
	}
	if carrierName != "" {
		spec, ok := carrierDivisors[strings.ToLower(carrierName)]
		if !ok {
			return 0, fmt.Errorf("unknown carrier %q, want one of: ups, fedex, usps", carrierName)
		}
		if unit == "cm" {
			return spec.metric, nil
		}
		return spec.imperial, nil
	}
	if unit == "cm" {
		return metricDivisor, nil
	}
	return imperialDivisor, nil
}

// billableWeight applies the carrier rounding rule: length, width, and
// height multiply out to cubic units, divide by the divisor to get dim
// weight, and both dim weight and actual weight round up independently
// before being compared, since that's how carriers bill fractional pounds.
func billableWeight(r record, divisor float64) (dimWeight, actual, billable float64, applies bool) {
	dimWeight = math.Ceil((r.length * r.width * r.height) / divisor)
	actual = math.Ceil(r.actualWeight)
	billable = actual
	if dimWeight > actual {
		billable = dimWeight
		applies = true
	}
	return dimWeight, actual, billable, applies
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

// jsonRecord mirrors the CSV columns for JSON input. Field names are
// lowercase to match the CSV header rather than following Go's usual
// exported-field casing, since this is what a user's JSON is expected
// to look like.
type jsonRecord struct {
	ID     string  `json:"id"`
	Length float64 `json:"length"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	Weight float64 `json:"weight"`
}

// readJSONRecords parses a JSON array of package objects. Unlike CSV
// there's no header-row ambiguity to resolve, so a malformed object
// fails the whole batch rather than being skipped.
func readJSONRecords(r io.Reader) ([]record, error) {
	var in []jsonRecord
	if err := json.NewDecoder(r).Decode(&in); err != nil {
		return nil, err
	}

	recs := make([]record, len(in))
	for i, jr := range in {
		recs[i] = record{
			id:           jr.ID,
			length:       jr.Length,
			width:        jr.Width,
			height:       jr.Height,
			actualWeight: jr.Weight,
		}
	}
	return recs, nil
}
