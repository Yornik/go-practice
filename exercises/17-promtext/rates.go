package promtext

import "time"

// Rates computes the per-second rate of every cumulative series between two
// scrapes of the same target, taken elapsed apart, like PromQL's irate()
// does with its last two samples. The result maps SeriesKey to the rate.
//
// Cumulative series are the samples of counter families and the _bucket,
// _sum and _count samples of histogram families (_sum and _count for
// summaries). Series missing from prev are skipped. A value lower than in
// prev is a counter reset. elapsed <= 0 is an error.
func Rates(prev, curr []Family, elapsed time.Duration) (map[string]float64, error) {
	panic("TODO: implement")
}
