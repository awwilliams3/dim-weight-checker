# dim-weight-checker

Carriers don't charge shipping based on actual weight alone. They charge
based on the greater of actual weight and *dimensional weight* (a
volumetric estimate: length x width x height, divided by a carrier-set
divisor). A big, light box can cost as much to ship as a small, heavy one.

This tool answers one question: for a batch of packages, what is the
billable weight, and did dimensional weight kick in?

## Input format

CSV rows of `id,length,width,height,weight`. A header row is fine, it's
detected automatically and skipped. Units are inches and pounds by
default, or centimeters and kilograms with `-unit cm`.

```
id,length,width,height,weight
box-1,12,10,8,4
box-2,24,18,18,6
envelope,15,12,1,0.5
```

With `-json`, input is a JSON array of objects with the same fields
instead:

```
[
  {"id": "box-1", "length": 12, "width": 10, "height": 8, "weight": 4},
  {"id": "box-2", "length": 24, "width": 18, "height": 18, "weight": 6}
]
```

## Usage

From a file:

```
dim-weight-checker packages.csv
```

From stdin:

```
cat packages.csv | dim-weight-checker
```

Multiple sources, mixing files and stdin (`-` means stdin):

```
dim-weight-checker warehouse-a.csv - warehouse-b.csv < warehouse-c-stream.csv
```

Output is CSV on stdout:

```
id,length,width,height,actual_weight,dim_weight,billable_weight,dim_applies
box-1,12,10,8,4,7,7,true
box-2,24,18,18,6,57,57,true
envelope,15,12,1,0.5,1,1,true
```

## Flags

- `-unit in|cm` — measurement system, default `in` (inches/pounds).
  `cm` means centimeters/kilograms.
- `-carrier ups|fedex|usps` — use that carrier's published divisor
  instead of the plain `-unit` default. UPS and FedEx both use 139
  (imperial) / 5000 (metric); USPS uses 166 (imperial) / 5000 (metric)
  for Priority Mail and Retail Ground. Unknown carrier names are an
  error rather than a silent fallback.
- `-divisor N` — override the dim weight divisor outright, taking
  precedence over both `-unit` and `-carrier`. Set this if your actual
  contract uses a different number, which does happen for
  international or freight rates.
- `-json` — parse input as a JSON array of objects instead of CSV.
  Applies to all sources for the run; you can't mix CSV and JSON files
  in one invocation.

## Why the defaults are what they are

139 and 5000 are the numbers UPS, FedEx, and USPS Priority Mail publish
for their standard domestic dimensional weight formula. They are common
enough to be a reasonable default, not universal. Always check your own
carrier's rate sheet before relying on this for a real invoice dispute.
