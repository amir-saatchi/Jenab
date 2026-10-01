package main

// Hand-written expected next runs. `want` follows Vixie/cronie day matching
// (DOM and DOW both restricted -> either matches; a field starting with '*' counts
// as unrestricted) and, for DST, the proposed rule A (see README).
// The brute-force reference must agree with every `want` (checked in main).

type tcase struct {
	group string
	expr  string
	zone  string
	from  string // RFC 3339 with the zone's offset (unambiguous in repeated hours)
	want  string // RFC 3339
	note  string
}

const (
	gBasic = "basic"
	gDays  = "dom+dow"
	gCal   = "calendar"
	gDST   = "dst"
)

var groups = []string{gBasic, gDays, gCal, gDST}

var cases = []tcase{
	// plain schedules, ranges, steps, lists, names, macros
	{gBasic, "* * * * *", "UTC", "2026-09-28T10:00:30Z", "2026-09-28T10:01:00Z", "from :30 seconds"},
	{gBasic, "0 * * * *", "UTC", "2026-09-28T10:00:00Z", "2026-09-28T11:00:00Z", "strictly after"},
	{gBasic, "5 4 * * *", "UTC", "2026-09-28T10:00:00Z", "2026-09-29T04:05:00Z", ""},
	{gBasic, "*/15 * * * *", "UTC", "2026-09-28T10:07:00Z", "2026-09-28T10:15:00Z", ""},
	{gBasic, "0-10/5 9 * * *", "UTC", "2026-09-28T09:06:00Z", "2026-09-28T09:10:00Z", ""},
	{gBasic, "0 9-17/4 * * *", "UTC", "2026-09-28T13:00:00Z", "2026-09-28T17:00:00Z", ""},
	{gBasic, "15,45 * * * *", "UTC", "2026-09-28T10:20:00Z", "2026-09-28T10:45:00Z", ""},
	{gBasic, "1-3,58-59 23 * * *", "UTC", "2026-09-28T23:03:00Z", "2026-09-28T23:58:00Z", ""},
	{gBasic, "0 0 1 * *", "UTC", "2026-09-28T10:00:00Z", "2026-10-01T00:00:00Z", ""},
	{gBasic, "0 0 1 */3 *", "UTC", "2026-09-28T10:00:00Z", "2026-10-01T00:00:00Z", "months 1,4,7,10"},
	{gBasic, "0 12 * JAN,JUL *", "UTC", "2026-09-28T10:00:00Z", "2027-01-01T12:00:00Z", ""},
	{gBasic, "0 12 * * MON-FRI", "UTC", "2026-10-02T12:00:00Z", "2026-10-05T12:00:00Z", "Friday to Monday"},
	{gBasic, "0 8 * * sun", "UTC", "2026-09-28T10:00:00Z", "2026-10-04T08:00:00Z", "lower-case name"},
	{gBasic, "0 8 * * 7", "UTC", "2026-09-28T10:00:00Z", "2026-10-04T08:00:00Z", "7 = Sunday"},
	{gBasic, "0 0 * * 1-5/2", "UTC", "2026-09-28T00:00:00Z", "2026-09-30T00:00:00Z", "Mon, Wed, Fri"},
	{gBasic, "@daily", "UTC", "2026-09-28T10:00:00Z", "2026-09-29T00:00:00Z", ""},
	{gBasic, "@hourly", "UTC", "2026-09-28T10:00:00Z", "2026-09-28T11:00:00Z", ""},
	{gBasic, "@weekly", "UTC", "2026-09-28T10:00:00Z", "2026-10-04T00:00:00Z", ""},
	{gBasic, "@monthly", "UTC", "2026-09-28T10:00:00Z", "2026-10-01T00:00:00Z", ""},
	{gBasic, "@yearly", "UTC", "2026-09-28T10:00:00Z", "2027-01-01T00:00:00Z", ""},
	{gBasic, "30 9 * * *", "Europe/Berlin", "2026-09-28T12:00:00+02:00", "2026-09-29T09:30:00+02:00", ""},
	{gBasic, "0 0 * * *", "Asia/Tehran", "2026-09-28T13:30:00+03:30", "2026-09-29T00:00:00+03:30", "+03:30 offset"},
	{gBasic, "0 9 * * 1-5", "America/New_York", "2026-09-26T10:00:00-04:00", "2026-09-28T09:00:00-04:00", "Saturday to Monday"},

	// day of month + day of week (from Monday 2026-09-28)
	{gDays, "0 0 13 * 5", "UTC", "2026-09-28T00:00:00Z", "2026-10-02T00:00:00Z", "13th OR Friday"},
	{gDays, "0 0 1 * MON", "UTC", "2026-09-28T00:00:00Z", "2026-10-01T00:00:00Z", "1st OR Monday"},
	{gDays, "0 0 1-7 * MON", "UTC", "2026-09-28T00:00:00Z", "2026-10-01T00:00:00Z", "not 'first Monday' in Vixie cron"},
	{gDays, "0 0 1,15 * 3", "UTC", "2026-09-28T00:00:00Z", "2026-09-30T00:00:00Z", ""},
	{gDays, "0 0 * * 5", "UTC", "2026-09-28T00:00:00Z", "2026-10-02T00:00:00Z", "DOW only"},
	{gDays, "0 0 13 * *", "UTC", "2026-09-28T00:00:00Z", "2026-10-13T00:00:00Z", "DOM only"},
	{gDays, "0 0 29 2 1", "UTC", "2026-09-28T00:00:00Z", "2027-02-01T00:00:00Z", "February only: 29th OR Monday"},
	{gDays, "0 0 */2 * 1", "UTC", "2026-09-28T00:00:00Z", "2026-10-05T00:00:00Z", "cronie quirk: DOM starts with * -> AND"},
	{gDays, "0 0 13 * */2", "UTC", "2026-09-28T00:00:00Z", "2026-10-13T00:00:00Z", "cronie quirk: DOW starts with * -> AND"},

	// leap years, end of month
	{gCal, "0 0 29 2 *", "UTC", "2026-09-28T00:00:00Z", "2028-02-29T00:00:00Z", ""},
	{gCal, "0 0 29 2 *", "UTC", "2028-02-29T00:00:00Z", "2032-02-29T00:00:00Z", ""},
	{gCal, "0 0 29 2 *", "UTC", "2096-03-01T00:00:00Z", "2104-02-29T00:00:00Z", "2100 is not a leap year: 8-year gap"},
	{gCal, "0 0 28-31 2 *", "UTC", "2027-02-28T00:00:00Z", "2028-02-28T00:00:00Z", ""},
	{gCal, "0 0 31 * *", "UTC", "2026-09-28T00:00:00Z", "2026-10-31T00:00:00Z", ""},
	{gCal, "0 0 31 * *", "UTC", "2026-10-31T00:00:00Z", "2026-12-31T00:00:00Z", "skips November"},
	{gCal, "0 0 30 * *", "UTC", "2027-01-30T00:00:00Z", "2027-03-30T00:00:00Z", "skips February"},
	{gCal, "59 23 31 12 *", "UTC", "2026-12-31T23:59:00Z", "2027-12-31T23:59:00Z", ""},
	{gCal, "0 0 1 1 *", "UTC", "2026-12-31T23:59:00Z", "2027-01-01T00:00:00Z", "year rollover"},

	// DST. Berlin: 2026-03-29 02:00 -> 03:00 (skipped), 2026-10-25 03:00 -> 02:00 (02:xx twice).
	// New York: 2026-03-08 02:00 -> 03:00, 2026-11-01 02:00 -> 01:00 (01:xx twice).
	{gDST, "30 2 * * *", "Europe/Berlin", "2026-03-28T12:00:00+01:00", "2026-03-29T03:00:00+02:00", "02:30 skipped"},
	{gDST, "30 2 * * *", "Europe/Berlin", "2026-03-29T03:00:00+02:00", "2026-03-30T02:30:00+02:00", "after the gap run"},
	{gDST, "0 2 * * *", "Europe/Berlin", "2026-03-28T12:00:00+01:00", "2026-03-29T03:00:00+02:00", "02:00 skipped"},
	{gDST, "*/15 * * * *", "Europe/Berlin", "2026-03-29T01:45:00+01:00", "2026-03-29T03:00:00+02:00", ""},
	{gDST, "0 * * * *", "Europe/Berlin", "2026-03-29T01:00:00+01:00", "2026-03-29T03:00:00+02:00", ""},
	{gDST, "*/15 2 * * *", "Europe/Berlin", "2026-03-29T01:00:00+01:00", "2026-03-30T02:00:00+02:00", "* job, whole hour skipped: no catch-up"},
	{gDST, "0,30 2 * * *", "Europe/Berlin", "2026-03-29T03:00:00+02:00", "2026-03-30T02:00:00+02:00", "two times in one gap run once"},
	{gDST, "0 12 * * *", "Europe/Berlin", "2026-03-29T00:00:00+01:00", "2026-03-29T12:00:00+02:00", "later on change day"},
	{gDST, "0 9 * * 1", "Europe/Berlin", "2026-03-27T12:00:00+01:00", "2026-03-30T09:00:00+02:00", "weekly across change"},
	{gDST, "30 2 * * *", "Europe/Berlin", "2026-10-24T12:00:00+02:00", "2026-10-25T02:30:00+02:00", "first 02:30"},
	{gDST, "30 2 * * *", "Europe/Berlin", "2026-10-25T02:30:00+02:00", "2026-10-26T02:30:00+01:00", "second 02:30 must not run"},
	{gDST, "*/15 * * * *", "Europe/Berlin", "2026-10-25T02:45:00+02:00", "2026-10-25T02:00:00+01:00", "repeated hour, * job runs again"},
	{gDST, "0 * * * *", "Europe/Berlin", "2026-10-25T02:00:00+02:00", "2026-10-25T02:00:00+01:00", "repeated hour, * job runs again"},
	{gDST, "30 2 * * *", "America/New_York", "2026-03-07T12:00:00-05:00", "2026-03-08T03:00:00-04:00", "02:30 skipped"},
	{gDST, "*/15 * * * *", "America/New_York", "2026-03-08T01:45:00-05:00", "2026-03-08T03:00:00-04:00", ""},
	{gDST, "0 * * * *", "America/New_York", "2026-03-08T01:00:00-05:00", "2026-03-08T03:00:00-04:00", ""},
	{gDST, "30 1 * * *", "America/New_York", "2026-10-31T12:00:00-04:00", "2026-11-01T01:30:00-04:00", "first 01:30"},
	{gDST, "30 1 * * *", "America/New_York", "2026-11-01T01:30:00-04:00", "2026-11-02T01:30:00-05:00", "second 01:30 must not run"},
	{gDST, "*/15 * * * *", "America/New_York", "2026-11-01T01:45:00-04:00", "2026-11-01T01:00:00-05:00", "repeated hour, * job runs again"},
	{gDST, "0 * * * *", "America/New_York", "2026-11-01T01:00:00-04:00", "2026-11-01T01:00:00-05:00", "repeated hour, * job runs again"},
	{gDST, "0 2 * * *", "America/New_York", "2026-11-01T00:00:00-04:00", "2026-11-01T02:00:00-05:00", "02:00 exists once"},
	{gDST, "0 0 * * *", "America/Santiago", "2026-09-05T12:00:00-04:00", "2026-09-06T01:00:00-03:00", "midnight skipped"},
	{gDST, "30 2 * * *", "Asia/Tehran", "2026-03-20T12:00:00+03:30", "2026-03-21T02:30:00+03:30", "no DST since 2022"},
	{gDST, "0 0 * * *", "Asia/Tehran", "2026-03-21T12:00:00+03:30", "2026-03-22T00:00:00+03:30", "old DST start day"},
	{gDST, "30 2 * * *", "UTC", "2026-03-29T00:00:00Z", "2026-03-29T02:30:00Z", ""},
	{gDST, "30 1 * * *", "UTC", "2026-11-01T00:00:00Z", "2026-11-01T01:30:00Z", ""},
}

// Part 3: expressions Burrow should reject (or accept, where noted).
type icase struct {
	expr   string
	accept bool // Burrow should accept it
	note   string
}

var invalid = []icase{
	{"60 * * * *", false, "minute out of range"},
	{"* 24 * * *", false, "hour out of range"},
	{"* * 0 * *", false, "day 0"},
	{"* * 32 * *", false, "day 32"},
	{"* * * 13 *", false, "month 13"},
	{"* * * * 8", false, "weekday 8"},
	{"* * 31 2 *", false, "never matches (31 Feb)"},
	{"0 0 30 2 *", false, "never matches (30 Feb)"},
	{"0 0 31 4,6,9,11 *", false, "never matches (31st of 30-day months)"},
	{"* * * * * *", false, "6 fields"},
	{"0 0 * * * 2027", false, "6 fields with year"},
	{"* * * *", false, "4 fields"},
	{"", false, "empty"},
	{"   ", false, "only spaces"},
	{"  0  9   *  * *  ", true, "extra spaces"},
	{"0\t9 * * *", true, "tab separator"},
	{"0 0 L * *", false, "L (last day)"},
	{"0 0 15W * *", false, "W (nearest weekday)"},
	{"0 0 * * 5#3", false, "# (third Friday)"},
	{"0 0 * * 5L", false, "L in weekday"},
	{"0 0 ? * MON", false, "? (Quartz)"},
	{"H * * * *", false, "Jenkins H"},
	{"0 22-2 * * *", false, "backwards range"},
	{"5-1 * * * *", false, "backwards range"},
	{"*/0 * * * *", false, "step 0"},
	{"5/15 * * * *", false, "step without range"},
	{"0 9 * * MON-FRI,", false, "trailing comma"},
	{"0 9 * * Monday", false, "full weekday name"},
	{"CRON_TZ=UTC 0 9 * * *", false, "zone prefix (zone is a separate field)"},
	{"TZ=Asia/Tehran 0 9 * * *", false, "zone prefix"},
	{"@reboot", false, "macro without a time"},
	{"@every 5m", false, "robfig interval"},
	{"@midnight", true, "cronie macro"},
}
