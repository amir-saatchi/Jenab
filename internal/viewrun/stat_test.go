package viewrun

import (
	"context"
	"testing"
)

// A stat needs exactly one row; its change keeps its sign.
func TestStat(t *testing.T) {
	ctx := context.Background()
	v := parse(t, `version: 1
id: coin_stat
title: Coin
type: stat
query: SELECT price, change FROM coins WHERE sym = :sym
params: { sym: ETH }
value: { field: price, format: currency, format_options: { currency: USD } }
change: { field: change, format: percent }
caption: Since yesterday
`)
	b := runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	s := b.Stat
	if s.Value != 3100.0 || s.Format != "currency" || s.Options.Currency != "USD" || s.Change != -0.01 || s.ChangeFormat != "percent" || s.Caption != "Since yesterday" {
		t.Errorf("%+v", s)
	}
	// NULL change: none.
	v.Params["sym"] = "XRP"
	b = runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	if b.Stat.Value != 0.6 || b.Stat.Change != nil {
		t.Errorf("%+v", b.Stat)
	}
	v.Params["sym"] = "DOGE"
	b = runner().View(ctx, v, ViewRequest{})
	if b.Error != "The query returned no rows; a stat needs exactly one." || b.Stat != nil {
		t.Errorf("no rows: %q", b.Error)
	}
	v.Query = "SELECT price, change FROM coins"
	b = runner().View(ctx, v, ViewRequest{})
	if b.Error != "The query returned 3 rows; a stat needs exactly one, as with LIMIT 1." {
		t.Errorf("three rows: %q", b.Error)
	}
	// SPEC 9.3's btc_today: ORDER BY and LIMIT, a change from a window.
	v = parse(t, `version: 1
id: btc_today
title: Today
type: stat
query: |
  SELECT price, price / LAG(price) OVER (ORDER BY day) - 1 AS change
  FROM prices WHERE coin = 'BTC' ORDER BY day DESC LIMIT 1
value: { field: price }
change: { field: change }
`)
	b = runner().View(ctx, v, ViewRequest{})
	noError(t, b)
	if c, ok := b.Stat.Change.(float64); !ok || c <= 0 || b.Stat.Value != 93000.0 {
		t.Errorf("btc_today %+v", b.Stat)
	}
}
