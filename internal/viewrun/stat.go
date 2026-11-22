package viewrun

import (
	"context"
	"fmt"

	"github.com/amir-saatchi/jenab/internal/view"
)

// Stat is what a stat view shows (SPEC 5.10). It maps onto the kit's
// StatSpec field for field (Format and ChangeFormat as CellFormat).
type Stat struct {
	Value   any // as Format needs it (value); nil for NULL, NaN or an infinity
	Format  string
	Options FormatOptions
	// Change is how the value changed, a signed number (an int64 or a
	// float64) when it is one; nil without a change field, or for NULL, NaN
	// or an infinity.
	Change       any
	ChangeFormat string
	Caption      string
}

// stat runs a stat view: exactly one row, else an error on the block.
func (r *Runner) stat(ctx context.Context, v *view.View, s *view.Stat, args []any) (*Stat, error) {
	// Two rows tell "more than one" without counting them all.
	rows, err := r.Source.ViewQuery(ctx, v.Query, args, 2)
	if err != nil {
		return nil, err
	}
	switch n := rows.Total; {
	case n == 0:
		return nil, fmt.Errorf("the query returned no rows; a stat needs exactly one")
	case n > 1 || rows.Capped:
		return nil, fmt.Errorf("the query returned %d rows; a stat needs exactly one, as with LIMIT 1", n)
	}
	fields := []string{s.Value.Field}
	if s.Change != nil {
		fields = append(fields, s.Change.Field)
	}
	idx, err := columnIndex(rows.Columns, fields...)
	if err != nil {
		return nil, err
	}
	row := rows.Values[0]
	out := &Stat{Format: s.Value.Format, Options: formatOptions(s.Value.FormatOptions), Caption: s.Caption}
	// NaN and the infinities show as missing, as NULL does.
	out.Value = finite(value(cmpOr(out.Format, FormatNumber), row[idx[0]]))
	if s.Change != nil {
		out.ChangeFormat = s.Change.Format
		out.Change = finite(value(cmpOr(out.ChangeFormat, FormatNumber), row[idx[1]]))
	}
	return out, nil
}
