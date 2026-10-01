package main

import (
	"time"

	"github.com/adhocore/gronx"
	"github.com/hashicorp/cronexpr"
	nr "github.com/netresearch/go-cron"
	robfig "github.com/robfig/cron/v3"
)

// nextFunc returns the first run strictly after t. Zero time means "none found".
type nextFunc func(t time.Time) (time.Time, error)

type lib struct {
	name  string
	short string
	parse func(expr string) (nextFunc, error)
}

var nrOrParser = nr.NewParser(nr.Minute | nr.Hour | nr.Dom | nr.Month | nr.Dow | nr.Descriptor | nr.DowOrDom)

// Every library gets t already converted to the pipeline's zone (t.In(loc)),
// which is how Burrow would call it; none of them gets a CRON_TZ prefix.
var libs = []lib{
	{"netresearch/go-cron v0.16.1 (ParseStandard, default AND)", "go-cron", func(e string) (nextFunc, error) {
		s, err := nr.ParseStandard(e)
		if err != nil {
			return nil, err
		}
		return func(t time.Time) (time.Time, error) { return s.Next(t), nil }, nil
	}},
	{"netresearch/go-cron v0.16.1 (NewParser with DowOrDom)", "go-cron OR", func(e string) (nextFunc, error) {
		s, err := nrOrParser.Parse(e)
		if err != nil {
			return nil, err
		}
		return func(t time.Time) (time.Time, error) { return s.Next(t), nil }, nil
	}},
	{"adhocore/gronx v1.20.5 (IsValid + NextTickAfter)", "gronx", func(e string) (nextFunc, error) {
		if !gronx.IsValid(e) {
			return nil, errInvalid
		}
		return func(t time.Time) (time.Time, error) { return gronx.NextTickAfter(e, t, false) }, nil
	}},
	{"robfig/cron/v3 v3.0.1 (ParseStandard)", "robfig", func(e string) (nextFunc, error) {
		s, err := robfig.ParseStandard(e)
		if err != nil {
			return nil, err
		}
		return func(t time.Time) (time.Time, error) { return s.Next(t), nil }, nil
	}},
	{"hashicorp/cronexpr v1.1.3 (Parse)", "cronexpr", func(e string) (nextFunc, error) {
		x, err := cronexpr.Parse(e)
		if err != nil {
			return nil, err
		}
		return func(t time.Time) (time.Time, error) { return x.Next(t), nil }, nil
	}},
}

type invalidErr struct{}

func (invalidErr) Error() string { return "IsValid returned false" }

var errInvalid = invalidErr{}
