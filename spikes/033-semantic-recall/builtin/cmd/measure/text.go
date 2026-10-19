package main

import (
	"math/rand/v2"
	"strings"
)

// Synthetic sentences per language. Pages are built by drawing from these
// with a fixed seed, so every run embeds the same text.
var sentences = map[string][]string{
	"en": {
		"The tenant pays the rent on the first working day of each month.",
		"Either party may end this agreement with three months' notice in writing.",
		"The deposit is returned within thirty days after the keys are handed back.",
		"Small repairs up to 150 euros per case are paid by the tenant.",
		"The landlord keeps the right to inspect the flat once a year after notice.",
		"Pets need the landlord's written consent, except for small animals in cages.",
		"The service charges cover water, heating, rubbish collection and the lift.",
		"Our team met on Tuesday to plan the release of version 2.4 of the app.",
		"The build failed because the test for the date parser timed out at 30 seconds.",
		"Please send the report to the finance team before Friday at noon.",
		"The new office opens in March, with room for forty people and two meeting rooms.",
		"Customers in Germany see prices in euros, while customers in the US see dollars.",
		"The server logs show 512 requests per second at the peak of the evening.",
		"We moved the database to a faster disk, and queries now take half the time.",
		"The training starts at nine and ends with a short test after lunch.",
		"Every employee gets twenty-eight days of paid leave per calendar year.",
		"Overtime is paid at the normal hourly rate plus twenty-five percent.",
		"The probation period lasts six months, during which two weeks' notice applies.",
		"Travel costs are refunded when the receipts arrive within four weeks.",
		"The project plan lists three milestones: design, build and the final review.",
		"A short summary of the meeting is in the shared folder under notes/2026-10.",
		"The recipe needs two cups of flour, one egg and a pinch of salt.",
		"Rain is expected over the weekend, so the garden party moves indoors.",
		"The library closes at eight on weekdays and at four on Saturdays.",
	},
	"de": {
		"Der Mieter zahlt die Miete am ersten Werktag jedes Monats.",
		"Jede Partei kann diesen Vertrag mit einer Frist von drei Monaten schriftlich kündigen.",
		"Die Kaution wird innerhalb von dreißig Tagen nach Rückgabe der Schlüssel erstattet.",
		"Kleinreparaturen bis 150 Euro je Fall trägt der Mieter.",
		"Der Vermieter darf die Wohnung einmal im Jahr nach Ankündigung besichtigen.",
		"Haustiere brauchen die schriftliche Zustimmung des Vermieters, außer Kleintiere im Käfig.",
		"Die Nebenkosten umfassen Wasser, Heizung, Müllabfuhr und den Aufzug.",
		"Unser Team hat sich am Dienstag getroffen, um die Version 2.4 der App zu planen.",
		"Der Build ist fehlgeschlagen, weil der Test für den Datumsparser nach 30 Sekunden abbrach.",
		"Bitte schicken Sie den Bericht bis Freitagmittag an die Buchhaltung.",
		"Das neue Büro öffnet im März, mit Platz für vierzig Menschen und zwei Besprechungsräumen.",
		"Kunden in Deutschland sehen Preise in Euro, Kunden in den USA in Dollar.",
		"Die Serverprotokolle zeigen am Abend bis zu 512 Anfragen pro Sekunde.",
		"Wir haben die Datenbank auf eine schnellere Platte verschoben, die Abfragen dauern nur noch halb so lange.",
		"Die Schulung beginnt um neun Uhr und endet nach dem Mittagessen mit einem kurzen Test.",
		"Jeder Mitarbeiter hat achtundzwanzig Tage bezahlten Urlaub pro Kalenderjahr.",
		"Überstunden werden mit dem normalen Stundenlohn plus fünfundzwanzig Prozent vergütet.",
		"Die Probezeit dauert sechs Monate, in denen eine Kündigungsfrist von zwei Wochen gilt.",
		"Reisekosten werden erstattet, wenn die Belege innerhalb von vier Wochen eingehen.",
		"Der Projektplan nennt drei Meilensteine: Entwurf, Umsetzung und die Abnahme.",
		"Eine kurze Zusammenfassung des Treffens liegt im gemeinsamen Ordner unter notes/2026-10.",
		"Für das Rezept braucht man zwei Tassen Mehl, ein Ei und eine Prise Salz.",
		"Am Wochenende soll es regnen, deshalb findet das Gartenfest drinnen statt.",
		"Die Bibliothek schließt werktags um acht und samstags um vier.",
	},
	"fa": {
		"مستأجر اجاره را در نخستین روز کاری هر ماه می‌پردازد.",
		"هر یک از طرفین می‌تواند این قرارداد را با سه ماه اطلاع کتبی فسخ کند.",
		"ودیعه ظرف سی روز پس از تحویل کلیدها بازگردانده می‌شود.",
		"تعمیرات جزئی تا ۱۵۰ یورو برای هر مورد بر عهده مستأجر است.",
		"موجر حق دارد سالی یک بار پس از اطلاع قبلی از آپارتمان بازدید کند.",
		"نگهداری حیوان خانگی به رضایت کتبی موجر نیاز دارد، مگر حیوانات کوچک در قفس.",
		"هزینه‌های جانبی شامل آب، گرمایش، جمع‌آوری زباله و آسانسور است.",
		"تیم ما سه‌شنبه جلسه گذاشت تا انتشار نسخه ۲٫۴ برنامه را برنامه‌ریزی کند.",
		"ساخت برنامه شکست خورد، چون آزمون تجزیه‌گر تاریخ پس از ۳۰ ثانیه متوقف شد.",
		"لطفاً گزارش را تا ظهر جمعه برای واحد مالی بفرستید.",
		"دفتر جدید در اسفند باز می‌شود و جای چهل نفر و دو اتاق جلسه دارد.",
		"مشتریان در آلمان قیمت‌ها را به یورو می‌بینند و مشتریان آمریکا به دلار.",
		"گزارش‌های سرور در اوج شب تا ۵۱۲ درخواست در ثانیه را نشان می‌دهند.",
		"پایگاه داده را به دیسک سریع‌تری منتقل کردیم و پرس‌وجوها اکنون نصف زمان می‌برند.",
		"دوره آموزشی ساعت نه شروع می‌شود و پس از ناهار با یک آزمون کوتاه پایان می‌یابد.",
		"هر کارمند در هر سال بیست‌وهشت روز مرخصی با حقوق دارد.",
		"اضافه‌کاری با نرخ عادی ساعتی به‌علاوه بیست‌وپنج درصد پرداخت می‌شود.",
		"دوره آزمایشی شش ماه است و در این مدت مهلت فسخ دو هفته است.",
		"هزینه‌های سفر در صورتی بازپرداخت می‌شود که رسیدها ظرف چهار هفته برسند.",
		"برنامه پروژه سه مرحله دارد: طراحی، ساخت و بازبینی نهایی.",
		"خلاصه کوتاه جلسه در پوشه مشترک زیر notes/2026-10 قرار دارد.",
		"برای این دستور پخت دو پیمانه آرد، یک تخم‌مرغ و کمی نمک لازم است.",
		"آخر هفته باران می‌آید، پس جشن باغ به داخل خانه منتقل می‌شود.",
		"کتابخانه روزهای کاری ساعت هشت و شنبه‌ها ساعت چهار بسته می‌شود.",
	},
}

// pageChars is the size of one page of text: about 500 English words.
const pageChars = 3000

// chunkChars is the size of one chunk sent to the model: about 300 tokens,
// under the spike's 400-token cap for every tokenizer tried.
const chunkChars = 1200

// chunks returns the chunks of n pages, split evenly over the three
// languages, each chunk a run of whole sentences of about chunkChars.
func chunks(pages int, seed uint64) []string {
	r := rand.New(rand.NewPCG(seed, seed^0x9e3779b97f4a7c15))
	langs := []string{"en", "de", "fa"}
	var out []string
	for p := range pages {
		pool := sentences[langs[p%len(langs)]]
		var page strings.Builder
		var chunk strings.Builder
		for page.Len() < pageChars {
			s := pool[r.IntN(len(pool))]
			if chunk.Len() > 0 {
				chunk.WriteByte(' ')
			}
			chunk.WriteString(s)
			page.WriteString(s)
			if chunk.Len() >= chunkChars {
				out = append(out, chunk.String())
				chunk.Reset()
			}
		}
		if chunk.Len() > 0 {
			out = append(out, chunk.String())
		}
	}
	return out
}

// queries returns short search queries in the three languages.
var queries = []string{
	"how long is the notice period for the flat?",
	"which currency do customers in Germany see?",
	"why did the build fail?",
	"Wie viele Urlaubstage gibt es?",
	"Wann wird die Kaution zurückgezahlt?",
	"Wo liegt die Zusammenfassung des Treffens?",
	"هزینه اضافه‌کاری چگونه محاسبه می‌شود؟",
	"دفتر جدید چه زمانی باز می‌شود؟",
	"چه کسی هزینه تعمیرات کوچک را می‌دهد؟",
}
